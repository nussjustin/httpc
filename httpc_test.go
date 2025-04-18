package httpc_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"
	"github.com/google/go-cmp/cmp"
	"github.com/nussjustin/problem"

	"github.com/nussjustin/httpc"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type infoResponse struct {
	Method string      `json:"method"`
	Host   string      `json:"host"`
	Path   string      `json:"path"`
	Query  url.Values  `json:"query"`
	Header http.Header `json:"header"`
	Body   string      `json:"body"`
}

func testEndpoint(tb testing.TB) (*http.Client, *url.URL) {
	tb.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		r.Header.Del("Accept-Encoding")
		r.Header.Del("Content-Length")
		r.Header.Del("User-Agent")

		resp := &infoResponse{
			Method: r.Method,
			Host:   r.Host,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header,
			Body:   string(body),
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if err := json.MarshalWrite(w, &resp); err != nil {
			panic(fmt.Errorf("failed to encode response: %w", err))
		}
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.StartTLS()

	tb.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	return srv.Client(), baseURL
}

func TestFetch(t *testing.T) {
	errTest := errors.New("test error")

	testCases := []struct {
		Name string

		Expected      infoResponse
		ExpectedError error

		Path   string
		Header http.Header

		Options []httpc.FetchOption
	}{
		{
			Name: "No options",
		},
		{
			Name: "WithPathValue",
			Expected: infoResponse{
				Path: "/A/B",
			},
			Path: "/{ValueA}/{ValueB}",
			Options: []httpc.FetchOption{
				httpc.WithPathValue("ValueA", "A"),
				httpc.WithPathValue("ValueB", "B"),
			},
		},
		{
			Name: "WithPathValue - multiple matches",
			Expected: infoResponse{
				Path: "/A/B/A/B",
			},
			Path: "/{ValueA}/{ValueB}/{ValueA}/{ValueB}",
			Options: []httpc.FetchOption{
				httpc.WithPathValue("ValueA", "A"),
				httpc.WithPathValue("ValueB", "B"),
			},
		},
		{
			Name: "WithPathValue - unknown key",
			Path: "/{ValueA}",
			ExpectedError: &httpc.UnusedPathValueError{
				URL:   &url.URL{Path: "/A"},
				Name:  "ValueB",
				Value: "B-1",
			},
			Options: []httpc.FetchOption{
				httpc.WithPathValue("ValueA", "A"),
				httpc.WithPathValue("ValueB", "B"),
			},
		},
		{
			Name: "WithAddedQueryParam",
			Expected: infoResponse{
				Query: url.Values{
					"Query-Param-A": []string{"A-1", "A-2"},
					"Query-Param-B": []string{"B-1", "B-2"},
				},
			},
			Options: []httpc.FetchOption{
				httpc.WithAddedQueryParam("Query-Param-A", "A-1"),
				httpc.WithAddedQueryParam("Query-Param-A", "A-2"),
				httpc.WithAddedQueryParam("Query-Param-B", "B-1"),
				httpc.WithAddedQueryParam("Query-Param-B", "B-2"),
			},
		},
		{
			Name: "WithQueryParam",
			Expected: infoResponse{
				Query: url.Values{
					"Query-Param-A": []string{"A-2"},
					"Query-Param-B": []string{"B-2"},
				},
			},
			Options: []httpc.FetchOption{
				httpc.WithQueryParam("Query-Param-A", "A-1"),
				httpc.WithQueryParam("Query-Param-A", "A-2"),
				httpc.WithQueryParam("Query-Param-B", "B-1"),
				httpc.WithQueryParam("Query-Param-B", "B-2"),
			},
		},
		{
			Name: "WithAddedHeader",
			Expected: infoResponse{
				Header: http.Header{
					"Header-A": []string{"A-1", "A-2"},
					"Header-B": []string{"B-1", "B-2"},
				},
			},
			Options: []httpc.FetchOption{
				httpc.WithAddedHeader("Header-A", "A-1"),
				httpc.WithAddedHeader("Header-A", "A-2"),
				httpc.WithAddedHeader("Header-B", "B-1"),
				httpc.WithAddedHeader("Header-B", "B-2"),
			},
		},
		{
			Name: "WithHeader",
			Expected: infoResponse{
				Header: http.Header{
					"Header-A": []string{"A-2"},
					"Header-B": []string{"B-2"},
				},
			},
			Options: []httpc.FetchOption{
				httpc.WithHeader("Header-A", "A-1"),
				httpc.WithHeader("Header-A", "A-2"),
				httpc.WithHeader("Header-B", "B-1"),
				httpc.WithHeader("Header-B", "B-2"),
			},
		},
		{
			Name: "WithBody",
			Expected: infoResponse{
				Body: "hello world",
			},
			Options: []httpc.FetchOption{
				httpc.WithBody(strings.NewReader("hello world")),
			},
		},
		{
			Name:          "WithBody - read error",
			ExpectedError: errTest,
			Options: []httpc.FetchOption{
				httpc.WithBody(&errorReader{errTest}),
			},
		},
		{
			Name: "WithBodyJSON",
			Expected: infoResponse{
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: `{"key":"value"}`,
			},
			Options: []httpc.FetchOption{
				httpc.WithBodyJSON(struct {
					Key string `json:"key"`
				}{"value"}, json.Deterministic(true)),
			},
		},
		{
			Name: "WithBodyJSON - custom Content-Type header",
			Expected: infoResponse{
				Header: http.Header{
					"Content-Type": []string{"application/my-content-type"},
				},
				Body: `{"key":"value"}`,
			},
			Options: []httpc.FetchOption{
				httpc.WithHeader("Content-Type", "application/my-content-type"),
				httpc.WithBodyJSON(struct {
					Key string `json:"key"`
				}{"value"}, json.Deterministic(true)),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			client, baseURL := testEndpoint(t)

			opts := append([]httpc.FetchOption{
				httpc.WithClient(client),
				httpc.WithBaseURL(baseURL),
			}, testCase.Options...)

			got, gotErr := httpc.Fetch[infoResponse](t.Context(), "GET", testCase.Path, opts...)

			switch {
			case testCase.ExpectedError == nil && gotErr == nil:
			case testCase.ExpectedError == nil && gotErr != nil:
				t.Errorf("got error %v, want nil", gotErr)
			case testCase.ExpectedError != nil && gotErr == nil:
				t.Errorf("got nil error, want %v", testCase.ExpectedError)
			case testCase.ExpectedError != nil && gotErr != nil:
				if gotErr.Error() != testCase.ExpectedError.Error() && !errors.Is(gotErr, testCase.ExpectedError) {
					t.Errorf("got error %v, want %v", gotErr, testCase.ExpectedError)
				}
			}

			if testCase.ExpectedError != nil || gotErr != nil {
				return
			}

			want := testCase.Expected
			want.Method = "GET"
			want.Host = baseURL.Host

			if want.Path == "" {
				want.Path = "/"
			}

			if want.Query == nil {
				want.Query = url.Values{}
			}

			if want.Header == nil {
				want.Header = http.Header{}
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Response mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFetch_Errors(t *testing.T) {
	testCases := []struct {
		Name     string
		Expected string
		Method   string
		Path     string
		Options  []httpc.FetchOption
	}{
		{
			Name:     "Invalid method",
			Expected: "net/http: invalid method \"HELLO WORLD\"",
			Method:   "HELLO WORLD",
			Path:     "/info",
		},
		{
			Name:     "Fetch option error",
			Expected: "json: cannot marshal from Go chan int",
			Method:   "GET",
			Path:     "/info",
			Options: []httpc.FetchOption{
				httpc.WithBodyJSON(make(chan int)),
			},
		},
		{
			Name:     "Failed request",
			Expected: "request failed",
			Method:   "GET",
			Path:     "/info",
			Options: []httpc.FetchOption{
				httpc.WithClient(&http.Client{
					Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return nil, errors.New("request failed")
					}),
				}),
			},
		},
		{
			Name:     "Response handler error",
			Expected: "handler error",
			Method:   "GET",
			Path:     "/info",
			Options: []httpc.FetchOption{
				httpc.WithHandler(httpc.ErrorHandler(errors.New("handler error"))),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			client, baseURL := testEndpoint(t)

			opts := append([]httpc.FetchOption{
				httpc.WithClient(client),
				httpc.WithBaseURL(baseURL),
			}, testCase.Options...)

			_, err := httpc.Fetch[infoResponse](t.Context(), testCase.Method, testCase.Path, opts...)
			if err == nil {
				t.Fatal("got nil-error")
			}

			if got, want := err.Error(), testCase.Expected; !strings.HasSuffix(got, want) {
				t.Errorf("got error %q, want %q", got, want)
			}
		})
	}
}

func TestHandlerChain(t *testing.T) {
	errTest := errors.New("test error")

	handler := func(text string, err error) httpc.HandlerFunc {
		return func(dst any, _ *http.Response) error {
			*dst.(*[]string) = append(*dst.(*[]string), text)
			return err
		}
	}

	testCases := []struct {
		Name          string
		Expected      []string
		ExpectedError error
		Handlers      []httpc.Handler
	}{
		{
			Name:          "Empty",
			ExpectedError: httpc.ErrUnhandledResponse,
		},
		{
			Name:     "Single handler",
			Expected: []string{"handler 1"},
			Handlers: []httpc.Handler{
				handler("handler 1", nil),
			},
		},
		{
			Name:     "Multiple handlers",
			Expected: []string{"handler 1"},
			Handlers: []httpc.Handler{
				handler("handler 1", nil),
				handler("handler 2", nil),
			},
		},
		{
			Name:     "Skipped handlers",
			Expected: []string{"handler 1", "handler 2", "handler 3"},
			Handlers: []httpc.Handler{
				handler("handler 1", httpc.ErrUnhandledResponse),
				handler("handler 2", httpc.ErrUnhandledResponse),
				handler("handler 3", nil),
				handler("handler 4", nil),
			},
		},
		{
			Name:          "Failed handler",
			Expected:      []string{"handler 1", "handler 2"},
			ExpectedError: errTest,
			Handlers: []httpc.Handler{
				handler("handler 1", httpc.ErrUnhandledResponse),
				handler("handler 2", errTest),
				handler("handler 3", nil),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			var got []string

			gotErr := httpc.HandlerChain(testCase.Handlers).HandleResponse(&got, nil)

			if diff := cmp.Diff(testCase.Expected, got); diff != "" {
				t.Errorf("dst mismatch (-want +got):\n%s", diff)
			}

			switch {
			case testCase.ExpectedError == nil && gotErr == nil:
			case testCase.ExpectedError != nil && gotErr == nil:
				t.Errorf("got no error, want %q", testCase.ExpectedError)
			case testCase.ExpectedError == nil && gotErr != nil:
				t.Errorf("got error %q, want no error", gotErr)
			case !errors.Is(gotErr, testCase.ExpectedError):
				t.Errorf("got error %q, want %q", gotErr, testCase.ExpectedError)
			}
		})
	}
}

func TestErrorHandler(t *testing.T) {
	want := errors.New("test error")

	h := httpc.ErrorHandler(want)

	if got := h(nil, &http.Response{}); !errors.Is(got, want) {
		t.Errorf("got error %v, want %v", got, want)
	}
}

type countingHandler struct {
	tb    testing.TB
	calls int
}

func newCountingHandler(tb testing.TB) *countingHandler {
	tb.Helper()

	return &countingHandler{tb: tb}
}

func (c *countingHandler) assertCalls(want int) {
	c.tb.Helper()

	if got := c.calls; got != want {
		c.tb.Errorf("got %d calls, want %d", got, want)
	}
}

func (c *countingHandler) HandleResponse(any, *http.Response) error {
	c.calls++
	return nil
}

func mustHandle(tb testing.TB, h httpc.Handler, dst any, resp *http.Response) {
	tb.Helper()

	if err := h.HandleResponse(dst, resp); err != nil {
		tb.Fatalf("failed to handle response: %v", err)
	}
}

func mustNotHandle(tb testing.TB, h httpc.Handler, dst any, resp *http.Response) {
	tb.Helper()

	if err := h.HandleResponse(dst, resp); !errors.Is(err, httpc.ErrUnhandledResponse) {
		tb.Fatalf("got error %v, want %v", err, httpc.ErrUnhandledResponse)
	}
}

func TestConditionalHandler(t *testing.T) {
	wrapped := newCountingHandler(t)

	var enabled bool

	handler := httpc.ConditionalHandler(func(*http.Response) bool { return enabled }, wrapped)

	mustNotHandle(t, handler, nil, &http.Response{StatusCode: http.StatusOK})
	wrapped.assertCalls(0)

	mustNotHandle(t, handler, nil, &http.Response{StatusCode: http.StatusOK})
	wrapped.assertCalls(0)

	enabled = true

	mustHandle(t, handler, nil, &http.Response{StatusCode: http.StatusOK})
	wrapped.assertCalls(1)

	mustHandle(t, handler, nil, &http.Response{StatusCode: http.StatusOK})
	wrapped.assertCalls(2)

	enabled = false

	mustNotHandle(t, handler, nil, &http.Response{StatusCode: http.StatusOK})
	wrapped.assertCalls(2)
}

func TestContentTypeHandler(t *testing.T) {
	wrapped := newCountingHandler(t)

	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Content-Type", "application/json")

	mustHandle(t, httpc.ContentTypeHandler("application/json", wrapped), nil, resp)
	wrapped.assertCalls(1)

	mustHandle(t, httpc.ContentTypeHandler("application/json", wrapped), nil, resp)
	wrapped.assertCalls(2)

	resp.Header.Set("Content-Type", "application/json; charset=utf-8")

	mustHandle(t, httpc.ContentTypeHandler("application/json", wrapped), nil, resp)
	wrapped.assertCalls(3)

	mustNotHandle(t, httpc.ContentTypeHandler("application/xml", wrapped), nil, resp)
	wrapped.assertCalls(3)
}

func TestDiscardBodyHandler(t *testing.T) {
	body := &readCloser{Reader: strings.NewReader("hello world")}

	resp := &http.Response{Body: body}

	if err := httpc.DiscardBodyHandler().HandleResponse(nil, resp); err != nil {
		t.Errorf("got error %v, want no error", err)
	}

	if !body.closed {
		t.Error("response body not closed")
	}

	b, err := io.ReadAll(body.Reader)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(b), 0; got != want {
		t.Errorf("got %d bytes, want %d", got, want)
	}
}

func TestDiscardBodyHandler_ReturnsErrorFromReader(t *testing.T) {
	body := &errorReader{err: errors.New("test error")}

	resp := &http.Response{Body: io.NopCloser(body)}

	if err := httpc.DiscardBodyHandler().HandleResponse(nil, resp); err == nil {
		t.Errorf("got no error, want %v", body.err)
	}
}

func TestJSONHandler(t *testing.T) {
	t.Run("Handled", func(t *testing.T) {
		body := &readCloser{
			Reader: strings.NewReader(`{"key1":"value1","key2":"value2"}`),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `json:"key1"`
			Key2 string `json:"key2"`
		}

		if err := httpc.UnmarshalJSONHandler()(&dst, resp); err != nil {
			t.Errorf("got error %v, want <nil>", err)
		}

		if got, want := dst.Key1, "value1"; got != want {
			t.Errorf("dst.Key1 = %v, want %v", got, want)
		}

		if got, want := dst.Key2, "value2"; got != want {
			t.Errorf("dst.Key2 = %v, want %v", got, want)
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})

	t.Run("Error", func(t *testing.T) {
		body := &readCloser{
			Reader: strings.NewReader(`{"key1":"value1","key2":"value2"`),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `json:"key1"`
			Key2 string `json:"key2"`
		}

		if err := httpc.UnmarshalJSONHandler()(&dst, resp); err == nil {
			t.Error("got nil error")
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})

	t.Run("Error when closing body", func(t *testing.T) {
		body := &readCloser{
			Reader:   strings.NewReader(`{"key1":"value1","key2":"value2"}`),
			closeErr: errors.New("close error"),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `json:"key1"`
			Key2 string `json:"key2"`
		}

		if got, want := httpc.UnmarshalJSONHandler()(&dst, resp), body.closeErr; !errors.Is(got, want) {
			t.Errorf("got error %v, want %v", got, want)
		}

		if got, want := dst.Key1, "value1"; got != want {
			t.Errorf("dst.Key1 = %v, want %v", got, want)
		}

		if got, want := dst.Key2, "value2"; got != want {
			t.Errorf("dst.Key2 = %v, want %v", got, want)
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})
}

func TestProblemHandler(t *testing.T) {
	t.Run("No problem", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		}

		want := httpc.ErrUnhandledResponse

		if got := httpc.ProblemHandler().HandleResponse(nil, resp); !errors.Is(got, want) {
			t.Errorf("got error %v, want %v", got, want)
		}
	})

	t.Run("Invalid problem", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Content-Type": []string{problem.ContentType},
			},
			Body: io.NopCloser(strings.NewReader(`invalid json`)),
		}

		want := "jsontext: invalid character 'i' at start of value"

		if got := httpc.ProblemHandler().HandleResponse(nil, resp); got.Error() != want {
			t.Errorf("got error %v, want %v", got, want)
		}
	})

	t.Run("Problem", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Content-Type": []string{problem.ContentType},
			},
			Body: io.NopCloser(strings.NewReader(`{"title":"some problem"}`)),
		}

		want := &problem.Details{Title: "some problem"}

		if got := httpc.ProblemHandler().HandleResponse(nil, resp); !cmp.Equal(want, got) {
			t.Errorf("got error %v, want %v", got, want)
		}
	})
}

func TestStatusHandler(t *testing.T) {
	wrapped := newCountingHandler(t)

	resp := &http.Response{StatusCode: http.StatusOK}

	mustHandle(t, httpc.StatusHandler(200, wrapped), nil, resp)
	wrapped.assertCalls(1)

	mustHandle(t, httpc.StatusHandler(200, wrapped), nil, resp)
	wrapped.assertCalls(2)

	mustNotHandle(t, httpc.StatusHandler(201, wrapped), nil, resp)
	wrapped.assertCalls(2)
}

func TestXMLHandler(t *testing.T) {
	t.Run("Handled", func(t *testing.T) {
		body := &readCloser{
			Reader: strings.NewReader(`<root><key1>value1</key1><key2>value2</key2></root>`),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `xml:"key1"`
			Key2 string `xml:"key2"`
		}

		if err := httpc.UnmarshalXMLHandler(false)(&dst, resp); err != nil {
			t.Errorf("got error %v, want <nil>", err)
		}

		if got, want := dst.Key1, "value1"; got != want {
			t.Errorf("dst.Key1 = %v, want %v", got, want)
		}

		if got, want := dst.Key2, "value2"; got != want {
			t.Errorf("dst.Key2 = %v, want %v", got, want)
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})

	t.Run("Error", func(t *testing.T) {
		body := &readCloser{
			Reader: strings.NewReader(`<root><key1>value1</key1><key2>value2</key2>`),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `xml:"key1"`
			Key2 string `xml:"key2"`
		}

		if err := httpc.UnmarshalXMLHandler(false)(&dst, resp); err == nil {
			t.Error("got nil error")
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})

	t.Run("Error when closing body", func(t *testing.T) {
		body := &readCloser{
			Reader:   strings.NewReader(`<root><key1>value1</key1><key2>value2</key2></root>`),
			closeErr: errors.New("close error"),
		}

		resp := &http.Response{Body: body}

		var dst struct {
			Key1 string `xml:"key1"`
			Key2 string `xml:"key2"`
		}

		if got, want := httpc.UnmarshalXMLHandler(false)(&dst, resp), body.closeErr; !errors.Is(got, want) {
			t.Errorf("got error %v, want %v", got, want)
		}

		if got, want := dst.Key1, "value1"; got != want {
			t.Errorf("dst.Key1 = %v, want %v", got, want)
		}

		if got, want := dst.Key2, "value2"; got != want {
			t.Errorf("dst.Key2 = %v, want %v", got, want)
		}

		if !body.closed {
			t.Error("body not closed")
		}
	})
}

type errorReader struct {
	err error
}

func (r *errorReader) Read([]byte) (n int, err error) {
	return 0, r.err
}

type readCloser struct {
	io.Reader
	closeErr error
	closed   bool
}

func (r *readCloser) Close() error {
	r.closed = true
	return r.closeErr
}
