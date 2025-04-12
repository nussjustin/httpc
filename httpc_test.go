package httpc_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/go-json-experiment/json"

	"github.com/nussjustin/httpc"
)

func TestNewRequest(t *testing.T) {
	req, err := httpc.NewRequest(t.Context(), "GET", "/",
		func(req *http.Request) error {
			req.Header.Add("Added-Header", "1")
			req.Header.Set("Replaced-Header", "1")
			return nil
		},
		func(req *http.Request) error {
			req.Header.Add("Added-Header", "2")
			req.Header.Set("Replaced-Header", "2")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := req.Method, "GET"; got != want {
		t.Errorf("Method = %q, want %q", got, want)
	}

	if got, want := req.URL.String(), "/"; got != want {
		t.Errorf("URL.String() = %q, want %q", got, want)
	}

	if got, want := req.Header["Added-Header"], []string{"1", "2"}; !slices.Equal(got, want) {
		t.Errorf("Header[\"Added-Header\"] = %v, want %v", got, want)
	}

	if got, want := req.Header["Replaced-Header"], []string{"2"}; !slices.Equal(got, want) {
		t.Errorf("Header[\"Replaced-Header\"] = %v, want %v", got, want)
	}
}

func TestNewRequest_WithInvalidURL(t *testing.T) {
	req, err := httpc.NewRequest(t.Context(), "GET", string([]byte{0}),
		func(*http.Request) error {
			panic("unreachable")
		},
	)
	if err == nil {
		t.Fatal("got no error")
	}

	if req != nil {
		t.Error("got non-nil request")
	}
}

func TestNewRequest_WithOptionError(t *testing.T) {
	req, err := httpc.NewRequest(t.Context(), "GET", "/",
		func(*http.Request) error {
			return errors.New("some error")
		},
		func(*http.Request) error {
			panic("unreachable")
		},
	)
	if err == nil {
		t.Fatal("got no error")
	}

	if got, want := err.Error(), "some error"; got != want {
		t.Errorf("err.Error() = %q, want %q", got, want)
	}

	if req != nil {
		t.Error("got non-nil request")
	}
}

func TestWithBaseURL(t *testing.T) {
	t.Run("Absolute Path", func(t *testing.T) {
		baseURL, err := url.Parse("https://example.com/api/")
		if err != nil {
			panic(err)
		}

		req := must(httpc.NewRequest(t.Context(), "GET", "/path/", httpc.WithBaseURL(baseURL)))

		if got, want := req.URL.String(), "https://example.com/path/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Relative Path", func(t *testing.T) {
		baseURL, err := url.Parse("https://example.com/api/")
		if err != nil {
			panic(err)
		}

		req := must(httpc.NewRequest(t.Context(), "GET", "path/", httpc.WithBaseURL(baseURL)))

		if got, want := req.URL.String(), "https://example.com/api/path/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})
}

func TestWithPathValue(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/:key",
			httpc.WithPathValue("key", "value")))

		if got, want := req.URL.String(), "/value"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Simple with slash", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/:key/",
			httpc.WithPathValue("key", "value")))

		if got, want := req.URL.String(), "/value/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("End", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/product/:id",
			httpc.WithPathValue("id", "1234")))

		if got, want := req.URL.String(), "/product/1234"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("End with slash", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/product/:id/",
			httpc.WithPathValue("id", "1234")))

		if got, want := req.URL.String(), "/product/1234/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Many", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/:entity1/:id1/:entity2/:id2",
			httpc.WithPathValue("entity1", "product"),
			httpc.WithPathValue("id1", "1234"),
			httpc.WithPathValue("entity2", "category"),
			httpc.WithPathValue("id2", "2345")))

		if got, want := req.URL.String(), "/product/1234/category/2345"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Duplicates", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/:key/between/:key/",
			httpc.WithPathValue("key", "value")))

		if got, want := req.URL.String(), "/value/between/value/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Error on missing", func(t *testing.T) {
		req, err := httpc.NewRequest(t.Context(), "GET", "/:key1/",
			httpc.WithPathValue("key1", "value1"),
			httpc.WithPathValue("key2", "value2"))
		if err == nil {
			t.Fatal("got no error")
		}

		var merr *httpc.MissingPathValueError
		if !errors.As(err, &merr) {
			t.Fatalf("got error of type %T, expected %T", err, merr)
		}

		if got, want := err.Error(), "placeholder :key2 not found in path /value1/"; got != want {
			t.Errorf("err.Error() = %q, want %q", got, want)
		}

		if req != nil {
			t.Error("got non-nil request")
		}
	})

	t.Run("No partial matches", func(t *testing.T) {
		req, err := httpc.NewRequest(t.Context(), "GET", "/product/1:key/",
			httpc.WithPathValue("key", "value"))
		if err == nil {
			t.Fatal("got no error")
		}

		var merr *httpc.MissingPathValueError
		if !errors.As(err, &merr) {
			t.Fatalf("got error of type %T, expected %T", err, merr)
		}

		if got, want := err.Error(), "placeholder :key not found in path /product/1:key/"; got != want {
			t.Errorf("err.Error() = %q, want %q", got, want)
		}

		if req != nil {
			t.Error("got non-nil request")
		}
	})

	t.Run("Only in path", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "https://:key@example.com/:key/",
			httpc.WithPathValue("key", "value")))

		if got, want := req.URL.String(), "https://:key@example.com/value/"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})
}

func TestWithAddedQueryParam(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/",
			httpc.WithAddedQueryParam("key", "value1"),
			httpc.WithAddedQueryParam("key", "value2")))

		if got, want := req.URL.String(), "/?key=value1&key=value2"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Existing", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/?key=value0&other=value",
			httpc.WithAddedQueryParam("key", "value1"),
			httpc.WithAddedQueryParam("key", "value2")))

		if got, want := req.URL.String(), "/?key=value0&key=value1&key=value2&other=value"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})
}

func TestWithQueryParam(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/",
			httpc.WithQueryParam("key", "value1"),
			httpc.WithQueryParam("key", "value2")))

		if got, want := req.URL.String(), "/?key=value2"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})

	t.Run("Existing", func(t *testing.T) {
		req := must(httpc.NewRequest(t.Context(), "GET", "/?key=value0&other=value",
			httpc.WithQueryParam("key", "value1"),
			httpc.WithQueryParam("key", "value2")))

		if got, want := req.URL.String(), "/?key=value2&other=value"; got != want {
			t.Errorf("URL.String() = %q, want %q", got, want)
		}
	})
}

func TestWithAddedHeader(t *testing.T) {
	req := must(httpc.NewRequest(t.Context(), "GET", "/",
		httpc.WithAddedHeader("Example", "value1"),
		httpc.WithAddedHeader("Example", "value2")))

	if got, want := req.Header["Example"], []string{"value1", "value2"}; !slices.Equal(got, want) {
		t.Errorf("Header[\"Example\"] = %v, want %v", got, want)
	}
}

func TestWithHeader(t *testing.T) {
	req := must(httpc.NewRequest(t.Context(), "GET", "/",
		httpc.WithHeader("Example", "value1"),
		httpc.WithHeader("Example", "value2")))

	if got, want := req.Header["Example"], []string{"value2"}; !slices.Equal(got, want) {
		t.Errorf("Header[\"Example\"] = %v, want %v", got, want)
	}
}

func TestWithAddedTrailer(t *testing.T) {
	req := must(httpc.NewRequest(t.Context(), "GET", "/",
		httpc.WithAddedTrailer("Example", "value1"),
		httpc.WithAddedTrailer("Example", "value2")))

	if got, want := req.Trailer["Example"], []string{"value1", "value2"}; !slices.Equal(got, want) {
		t.Errorf("Trailer[\"Example\"] = %v, want %v", got, want)
	}
}

func TestWithTrailer(t *testing.T) {
	req := must(httpc.NewRequest(t.Context(), "GET", "/",
		httpc.WithTrailer("Example", "value1"),
		httpc.WithTrailer("Example", "value2")))

	if got, want := req.Trailer["Example"], []string{"value2"}; !slices.Equal(got, want) {
		t.Errorf("Trailer[\"Example\"] = %v, want %v", got, want)
	}
}

func TestWithBody(t *testing.T) {
	t.Run("Known Type", func(t *testing.T) {
		r := strings.NewReader("hello world")

		req := must(httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithBody(r)))

		if got, want := req.ContentLength, int64(len("hello world")); got != want {
			t.Errorf("ContentLength = %d, want %d", got, want)
		}

		if got, want := read(t, req.Body), "hello world"; got != want {
			t.Errorf("Body = %q, want %q", got, want)
		}
	})

	t.Run("Unknown Type", func(t *testing.T) {
		r := struct{ io.Reader }{strings.NewReader("hello world")}

		req := must(httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithBody(r)))

		if got, want := req.ContentLength, int64(0); got != want {
			t.Errorf("ContentLength = %d, want %d", got, want)
		}

		if got, want := read(t, req.Body), "hello world"; got != want {
			t.Errorf("Body = %q, want %q", got, want)
		}
	})

	t.Run("Closer", func(t *testing.T) {
		r := &readCloser{Reader: strings.NewReader("hello world")}

		req := must(httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithBody(r)))

		if got, want := req.ContentLength, int64(0); got != want {
			t.Errorf("ContentLength = %d, want %d", got, want)
		}

		if got, want := read(t, req.Body), "hello world"; got != want {
			t.Errorf("Body = %q, want %q", got, want)
		}

		if err := req.Body.Close(); err != nil {
			t.Errorf("Body.Close() = %v, want <nil>", err)
		}

		if !r.closed {
			t.Error("reader not closed")
		}
	})
}

func TestWithJSON(t *testing.T) {
	t.Run("No existing Content-Type", func(t *testing.T) {
		value := map[string]any{"key1": "value1", "key2": "value2"}

		req := must(httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithJSON(value, json.Deterministic(true))))

		if got, want := req.Header["Content-Type"], []string{"application/json"}; !slices.Equal(got, want) {
			t.Errorf("Header[\"Content-Type\"] = %v, want %v", got, want)
		}

		if got, want := req.ContentLength, int64(len(`{"key1":"value1","key2":"value2"}`)); got != want {
			t.Errorf("ContentLength = %d, want %d", got, want)
		}

		if got, want := read(t, req.Body), `{"key1":"value1","key2":"value2"}`; got != want {
			t.Errorf("Body = %q, want %q", got, want)
		}
	})

	t.Run("Existing Content-Type", func(t *testing.T) {
		value := map[string]any{"key1": "value1", "key2": "value2"}

		req := must(httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithHeader("Content-Type", "application/test; some=value"),
			httpc.WithJSON(value, json.Deterministic(true))))

		if got, want := req.Header["Content-Type"], []string{"application/test; some=value"}; !slices.Equal(got, want) {
			t.Errorf("Header[\"Content-Type\"] = %v, want %v", got, want)
		}

		if got, want := req.ContentLength, int64(len(`{"key1":"value1","key2":"value2"}`)); got != want {
			t.Errorf("ContentLength = %d, want %d", got, want)
		}

		if got, want := read(t, req.Body), `{"key1":"value1","key2":"value2"}`; got != want {
			t.Errorf("Body = %q, want %q", got, want)
		}
	})

	t.Run("Error", func(t *testing.T) {
		req, err := httpc.NewRequest(t.Context(), "POST", "/",
			httpc.WithJSON(make(chan int)))
		if err == nil {
			t.Fatal("got no error")
		}

		var merr *json.SemanticError
		if !errors.As(err, &merr) {
			t.Fatalf("got error of type %T, expected %T", err, merr)
		}

		if req != nil {
			t.Error("got non-nil request")
		}
	})
}

type testResponse struct {
	Value string `json:"value"`
}

func testEndpoint(tb testing.TB) *httpc.Endpoint[testResponse] {
	tb.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		value := "value"

		if values := r.URL.Query()["value"]; len(values) > 0 {
			value = strings.Join(values, "&")
		}

		_ = json.MarshalWrite(w, &testResponse{Value: value})
	})

	server := httptest.NewUnstartedServer(mux)
	server.StartTLS()

	tb.Cleanup(func() {
		server.Close()
	})

	return &httpc.Endpoint[testResponse]{
		Client: server.Client(),
		Method: "GET",
		URL:    server.URL,
		Handlers: []httpc.Handler{
			httpc.JSONHandler(),
		},
	}
}

func TestEndpoint_Do(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		e := testEndpoint(t)

		v, resp := must2(e.Do(t.Context())) //nolint:bodyclose

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Errorf("resp.StatusCode = %v, want %v", got, want)
		}

		if got, want := v.Value, "value"; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}
	})

	t.Run("Endpoint options", func(t *testing.T) {
		e := testEndpoint(t)
		e.Options = []httpc.RequestOption{
			httpc.WithQueryParam("value", "value1"),
		}

		v, resp := must2(e.Do(t.Context())) //nolint:bodyclose

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Errorf("resp.StatusCode = %v, want %v", got, want)
		}

		if got, want := v.Value, "value1"; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}
	})

	t.Run("Endpoint and request options", func(t *testing.T) {
		e := testEndpoint(t)
		e.Options = []httpc.RequestOption{
			httpc.WithQueryParam("value", "value1"),
		}

		v, resp := must2(e.Do(t.Context(), httpc.WithAddedQueryParam("value", "value2"))) //nolint:bodyclose

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Errorf("resp.StatusCode = %v, want %v", got, want)
		}

		if got, want := v.Value, "value1&value2"; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}
	})

	t.Run("Request options", func(t *testing.T) {
		e := testEndpoint(t)

		v, resp := must2(e.Do(t.Context(), httpc.WithQueryParam("value", "value2"))) //nolint:bodyclose

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Errorf("resp.StatusCode = %v, want %v", got, want)
		}

		if got, want := v.Value, "value2"; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}
	})

	t.Run("Request creation error", func(t *testing.T) {
		e := testEndpoint(t)

		//nolint:bodyclose
		v, resp, err := e.Do(t.Context(), func(*http.Request) error {
			return io.EOF
		})

		if resp != nil {
			t.Error("got non-nil response")
		}

		if got, want := v.Value, ""; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}

		if !errors.Is(err, io.EOF) {
			t.Errorf("err = %v, want %v", err, io.EOF)
		}
	})

	t.Run("Request error", func(t *testing.T) {
		e := testEndpoint(t)

		//nolint:bodyclose
		v, resp, err := e.Do(t.Context(), func(req *http.Request) error {
			req.URL.Scheme = "bla"
			return nil
		})

		if resp != nil {
			t.Error("got non-nil response")
		}

		if got, want := v.Value, ""; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}

		if err == nil {
			t.Error("got nil error")
		}
	})

	t.Run("Handler error", func(t *testing.T) {
		e := testEndpoint(t)
		e.Handlers = []httpc.Handler{
			httpc.ErrorHandler(errors.ErrUnsupported),
		}

		v, resp, err := e.Do(t.Context()) //nolint:bodyclose

		if resp != nil {
			t.Error("got non-nil response")
		}

		if got, want := v.Value, ""; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}

		if got, want := err, errors.ErrUnsupported; !errors.Is(got, want) {
			t.Errorf("err = %v, want %v", got, want)
		}
	})

	t.Run("No handler", func(t *testing.T) {
		e := testEndpoint(t)
		e.Handlers = []httpc.Handler{}

		//nolint:bodyclose
		v, resp, err := e.Do(t.Context())

		if resp != nil {
			t.Error("got non-nil response")
		}

		if got, want := v.Value, ""; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}

		if got, want := err, httpc.ErrNotHandled; !errors.Is(got, want) {
			t.Errorf("err = %v, want %v", got, want)
		}
	})

	t.Run("No matching handler", func(t *testing.T) {
		e := testEndpoint(t)
		e.Handlers = []httpc.Handler{
			httpc.StatusHandler(http.StatusTeapot, httpc.ErrorHandler(errors.ErrUnsupported)),
		}

		//nolint:bodyclose
		v, resp, err := e.Do(t.Context())

		if resp != nil {
			t.Error("got non-nil response")
		}

		if got, want := v.Value, ""; got != want {
			t.Errorf("v.Value = %v, want %v", got, want)
		}

		if got, want := err, httpc.ErrNotHandled; !errors.Is(got, want) {
			t.Errorf("err = %v, want %v", got, want)
		}
	})
}

func TestErrorHandler(t *testing.T) {
	want := errors.New("test error")

	h := httpc.ErrorHandler(want)

	if got := h(nil, &http.Response{}); !errors.Is(got, want) {
		t.Errorf("err = %v, want %v", got, want)
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

		if err := httpc.JSONHandler()(&dst, resp); err != nil {
			t.Errorf("err = %v, want <nil>", err)
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

		if err := httpc.JSONHandler()(&dst, resp); err == nil {
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

		if got, want := httpc.JSONHandler()(&dst, resp), body.closeErr; !errors.Is(got, want) {
			t.Errorf("err = %v, want %v", got, want)
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

func TestStatusHandler(t *testing.T) {
	t.Run("Not skipped", func(t *testing.T) {
		var called bool

		h := httpc.StatusHandler(http.StatusOK, func(any, *http.Response) error {
			called = true
			return nil
		})

		resp := &http.Response{StatusCode: http.StatusOK}

		if err := h(nil, resp); err != nil {
			t.Errorf("err = %v, want <nil>", err)
		}

		if !called {
			t.Error("handler not called")
		}
	})

	t.Run("Skipped", func(t *testing.T) {
		var called bool

		h := httpc.StatusHandler(http.StatusOK, func(any, *http.Response) error {
			called = true
			return nil
		})

		resp := &http.Response{StatusCode: http.StatusNotFound}

		if got, want := h(nil, resp), httpc.ErrSkipHandler; !errors.Is(got, want) {
			t.Errorf("err = %v, want %v", got, want)
		}

		if called {
			t.Error("handler called")
		}
	})
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

func read(tb testing.TB, r io.Reader) string {
	tb.Helper()

	b, err := io.ReadAll(r)
	if err != nil {
		tb.Fatalf("failed to read body: %v", err)
	}

	return string(b)
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

func must2[T, U any](t T, u U, err error) (T, U) {
	if err != nil {
		panic(err)
	}
	return t, u
}
