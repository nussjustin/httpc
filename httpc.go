package httpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	neturl "net/url"
)

// RequestOption defines the signature for functions that can be used to configure a HTTP Request.
type RequestOption func(*http.Request) error

// NewRequest returns a new HTTP request for the given method and URL with the given options applied.
//
// It is equivalent to [http.NewRequestWithContext] followed by applying all options and returning on the first error.
func NewRequest(ctx context.Context, method string, url string, opts ...RequestOption) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		if err := opt(req); err != nil {
			return nil, err
		}
	}

	return req, nil
}

// WithBaseURL configures a request to use the given base URL:
//
// This can be useful for example when the paths are always the same but the domain may differ and allows for easier
// separation between those.
func WithBaseURL(baseURL *neturl.URL) RequestOption {
	return func(req *http.Request) error {
		req.URL = baseURL.ResolveReference(req.URL)
		return nil
	}
}

// MissingPathValueError is returned when a path value specified by [WithPathValue] is not found.
type MissingPathValueError struct {
	// URL is the URL as it was at the time the error occurred.
	URL neturl.URL

	// Name is the name of the path value as given to [WithPathValue]
	Name string

	// Value is the value of the path value as given to [WithPathValue]
	Value string
}

// Error implements the [error] interface.
func (e *MissingPathValueError) Error() string {
	return fmt.Sprintf("placeholder :%s not found in path %s", e.Name, e.URL.Path)
}

// WithPathValue searches the URLs path for path values with the given key and replaces them with the given value.
//
// The syntax used is the same as when registering routes with [http.ServeMux] and uses a colon before the param name.
//
// For example given the path "/api/product/:id", calling WithPathValue with name "id" and value "1234" will change the
// path to "/api/product/1234".
//
// The value will automatically be escaped using [url.PathEscape].
//
// There must not be any characters before the colon other than a slash, otherwise the value is not replaced. For
// example "/api/product/p:id" would not work as there is a "p" before the ":id".
//
// If no path value with the given name is found, a [MissingPathValueError] is returned.
func WithPathValue(name string, value string) RequestOption {
	pattern := regexp.MustCompile(fmt.Sprintf(`(^|/):%s(/|$)`, regexp.QuoteMeta(name)))

	return func(req *http.Request) error {
		replaced := pattern.ReplaceAllString(req.URL.Path, "${1}"+neturl.PathEscape(value)+"${2}")

		if req.URL.Path == replaced {
			return &MissingPathValueError{URL: *req.URL, Name: name, Value: value}
		}

		req.URL.Path = replaced
		return nil
	}
}

// WithAddedQueryParam adds a query parameter.
//
// Existing values are kept and the new value is added after them.
func WithAddedQueryParam(key, value string) RequestOption {
	return func(req *http.Request) error {
		q := req.URL.Query()
		q.Add(key, value)
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

// WithQueryParam sets a query parameter.
//
// Any existing values for the parameter are replaced.
func WithQueryParam(key, value string) RequestOption {
	return func(req *http.Request) error {
		q := req.URL.Query()
		q.Set(key, value)
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

// WithAddedHeader adds a header parameter.
//
// Existing values are kept and the new value is added after them.
func WithAddedHeader(key, value string) RequestOption {
	return func(req *http.Request) error {
		req.Header.Add(key, value)
		return nil
	}
}

// WithHeader sets a header.
//
// Any existing values for the header are replaced.
func WithHeader(key, value string) RequestOption {
	return func(req *http.Request) error {
		req.Header.Set(key, value)
		return nil
	}
}

// WithAddedTrailer adds a trailer parameter.
//
// Existing values are kept and the new value is added after them.
func WithAddedTrailer(key, value string) RequestOption {
	return func(req *http.Request) error {
		if req.Trailer == nil {
			req.Trailer = make(http.Header)
		}
		req.Trailer.Add(key, value)
		return nil
	}
}

// WithTrailer sets a trailer.
//
// Any existing values for the trailer are replaced.
func WithTrailer(key, value string) RequestOption {
	return func(req *http.Request) error {
		if req.Trailer == nil {
			req.Trailer = make(http.Header)
		}
		req.Trailer.Set(key, value)
		return nil
	}
}

func asReadCloser(r io.Reader) io.ReadCloser {
	rc, ok := r.(io.ReadCloser)
	if !ok {
		return io.NopCloser(r)
	}
	return rc
}

// WithBody sets the body for the request to the given io.Reader.
//
// If the given reader is either a [*bytes.Buffer], [*bytes.Reader] or [*strings.Reader] it will also set the content
// length to number of bytes available.
func WithBody(body io.Reader) RequestOption {
	return func(req *http.Request) error {
		switch v := body.(type) {
		case *bytes.Buffer:
			req.ContentLength = int64(v.Len())
		case *bytes.Reader:
			req.ContentLength = int64(v.Len())
		case *strings.Reader:
			req.ContentLength = int64(v.Len())
		}

		req.Body = asReadCloser(body)
		return nil
	}
}

// WithJSON encodes the given value as JSON and uses the result as the request body.
//
// If the Content-Type header is not set or empty, it will be set to "application/json".
func WithJSON(v any, opts ...jsontext.Options) RequestOption {
	return func(req *http.Request) error {
		body, err := json.Marshal(v, opts...)
		if err != nil {
			return err
		}

		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		req.ContentLength = int64(len(body))
		req.Body = io.NopCloser(bytes.NewReader(body))

		return nil
	}
}

// Endpoint defines an HTTP endpoint and allows requesting the endpoint and automatically returning the typed response.
type Endpoint[T any] struct {
	// Client is used to execute HTTP requests.
	//
	// If nil, [http.DefaultClient] is used.
	Client *http.Client

	// Method is the HTTP method used for the request.
	Method string

	// URL is the raw URL of the endpoint.
	URL string

	// Options can contain zero or more options which are used to modify requests.
	Options []RequestOption

	// Handlers contains a list of functions to handle the response.
	Handlers []Handler
}

// Do sends a request to the endpoint and returns the response.
//
// The response body will always be closed even if not handled.
func (e *Endpoint[T]) Do(ctx context.Context, opts ...RequestOption) (T, *http.Response, error) {
	var mergedOpts []RequestOption

	switch {
	case len(e.Options) == 0:
		mergedOpts = opts
	case len(opts) == 0:
		mergedOpts = e.Options
	default:
		mergedOpts = append(slices.Clip(e.Options), opts...)
	}

	req, err := NewRequest(ctx, e.Method, e.URL, mergedOpts...)
	if err != nil {
		var zeroT T
		return zeroT, nil, err
	}

	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		var zeroT T
		return zeroT, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var t T
	var handled bool

	for _, h := range e.Handlers {
		handlerErr := h(&t, resp)

		if handlerErr == nil {
			handled = true
			break
		}

		if errors.Is(handlerErr, ErrSkipHandler) {
			continue
		}

		var zeroT T
		return zeroT, nil, handlerErr
	}

	if !handled {
		var zeroT T
		return zeroT, nil, ErrNotHandled
	}

	return t, resp, nil
}

// Handler specifies the signature for functions that can handle a HTTP response.
//
// The dst value is a pointer to a value into which the response body should be written.
type Handler func(dst any, resp *http.Response) error

var (
	// ErrNotHandled is returned by [Endpoint.Do] if no [Handler] was able to handle the response.
	ErrNotHandled = errors.New("not handled")

	// ErrSkipHandler can be returned by [Handler]s when they can not process the response, causing the next handler to
	// be executed.
	ErrSkipHandler = errors.New("handler skipped")
)

// ErrorHandler returns a [Handler] that returns the given error.
func ErrorHandler(err error) Handler {
	return func(any, *http.Response) error {
		return err
	}
}

// JSONHandler returns a [Handler] that decodes the response body as JSON.
func JSONHandler(opts ...jsontext.Options) Handler {
	return func(dst any, resp *http.Response) (err error) {
		defer func() {
			if cErr := resp.Body.Close(); cErr != nil && err == nil {
				err = cErr
			}
		}()

		return json.UnmarshalRead(resp.Body, dst, opts...)
	}
}

// StatusHandler returns a [Handler] that checks the status code and either forwards the handling to the given handler
// or, if the response status does not match the given one, returns [ErrSkipHandler].
func StatusHandler(statusCode int, handler Handler) Handler {
	return func(dst any, resp *http.Response) error {
		if resp.StatusCode != statusCode {
			return ErrSkipHandler
		}
		return handler(dst, resp)
	}
}
