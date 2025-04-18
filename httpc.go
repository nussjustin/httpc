package httpc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/nussjustin/problem"
)

type fetchContext struct {
	// Client is the underlying client used for making requests.
	//
	// Defaults to [http.DefaultClient].
	Client *http.Client

	// Request contains the raw request that will be made.
	Request *http.Request

	// Handler is called to handle the response.
	//
	// Defaults to [DefaultHandlers].
	Handler Handler
}

// DefaultHandlers is the default [Handler] used by [Fetch] if no other [Handler] was specified.
//
// It will automatically handle RFC 9457 style errors, JSON and XML responses as well as 204 and 304 responses.
var DefaultHandlers = HandlerChain{
	ProblemHandler(),
	ContentTypeHandler("application/json", UnmarshalJSONHandler()),
	ContentTypeHandler("application/xml", UnmarshalXMLHandler(false)),
	StatusHandler(http.StatusNoContent, DiscardBodyHandler()),
	StatusHandler(http.StatusNotModified, DiscardBodyHandler()),
}

// FetchOption defines the signature for functions that can be used to configure the request creation and response
// handling of [Fetch].
type FetchOption func(*fetchContext) error

// Fetch requests the given endpoint and returns the parsed response.
//
// The request and the response handling can be customized by passing different options.
//
// In order to access the original response data, use [FetchWithResponse] instead.
func Fetch[T any](ctx context.Context, method string, url string, opts ...FetchOption) (T, error) {
	t, resp, err := FetchWithResponse[T](ctx, method, url, opts...)
	if resp != nil {
		defer func() { _ = discardBody(resp) }()
	}
	return t, err
}

// FetchWithResponse is the same as [Fetch], but also returns the raw response.
//
// Depending on the used [Handler], the response body may already be closed.
//
// If the response was already received, it will be returned even on error.
func FetchWithResponse[T any](
	ctx context.Context,
	method string,
	url string,
	opts ...FetchOption,
) (T, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		var zeroT T
		return zeroT, nil, err
	}

	fetchCtx := &fetchContext{Client: http.DefaultClient, Request: req, Handler: DefaultHandlers}

	for _, opt := range opts {
		if err := opt(fetchCtx); err != nil {
			var zeroT T
			return zeroT, nil, err
		}
	}

	resp, err := fetchCtx.Client.Do(req)
	if err != nil {
		var zeroT T
		return zeroT, resp, err
	}

	var t T

	if err := fetchCtx.Handler.HandleResponse(&t, resp); err != nil {
		var zeroT T
		return zeroT, resp, err
	}

	return t, resp, nil
}

// WithClient sets the underlying client used by [Fetch] to make the request and receive the response.
func WithClient(client *http.Client) FetchOption {
	return func(fetchCtx *fetchContext) error {
		fetchCtx.Client = client
		return nil
	}
}

// WithBaseURL configures a request to use the given base URL.
//
// This can be useful for example when the paths are always the same but the domain may differ and allows for easier
// separation between those.
func WithBaseURL(baseURL *url.URL) FetchOption {
	return func(ctx *fetchContext) error {
		ctx.Request.URL = baseURL.ResolveReference(ctx.Request.URL)
		return nil
	}
}

// UnusedPathValueError is returned when a path value specified by [WithPathValue] is not found in the path.
type UnusedPathValueError struct {
	// URL is the URL as it was at the time the error occurred.
	URL *url.URL

	// Name is the name of the path value as given to [WithPathValue]
	Name string

	// Value is the value of the path value as given to [WithPathValue]
	Value string
}

// Error implements the [error] interface.
func (e *UnusedPathValueError) Error() string {
	return fmt.Sprintf("placeholder {%s} not found in path %s", e.Name, e.URL.Path)
}

// WithPathValue searches the URLs path for path values with the given key and replaces them with the given value.
//
// The syntax used is the same as when registering routes with [http.ServeMux].
//
// For example given the path "/api/product/{id}", calling WithPathValue with name "id" and value "1234" will result in
// the path "/api/product/1234".
//
// The value will automatically be escaped using [url.PathEscape].
//
// There must not be any characters before the colon other than a slash, otherwise the value is not replaced. For
// example "/api/product/p{id}" would not work as there is a "p" before the {id}.
//
// If no path value with the given name is found, a [UnusedPathValueError] is returned.
func WithPathValue(name string, value string) FetchOption {
	pattern := regexp.MustCompile(fmt.Sprintf(`(^|/)\{%s\}(/|$)`, regexp.QuoteMeta(name)))

	return func(ctx *fetchContext) error {
		replaced := pattern.ReplaceAllString(ctx.Request.URL.Path, "${1}"+url.PathEscape(value)+"${2}")

		if ctx.Request.URL.Path == replaced {
			return &UnusedPathValueError{URL: ctx.Request.URL, Name: name, Value: value}
		}

		ctx.Request.URL.Path = replaced
		return nil
	}
}

// WithAddedQueryParam adds a query parameter.
//
// Existing values are kept and the new value is added after them.
func WithAddedQueryParam(key, value string) FetchOption {
	return func(ctx *fetchContext) error {
		q := ctx.Request.URL.Query()
		q.Add(key, value)
		ctx.Request.URL.RawQuery = q.Encode()
		return nil
	}
}

// WithQueryParam sets a query parameter.
//
// Any existing values for the parameter are replaced.
func WithQueryParam(key, value string) FetchOption {
	return func(ctx *fetchContext) error {
		q := ctx.Request.URL.Query()
		q.Set(key, value)
		ctx.Request.URL.RawQuery = q.Encode()
		return nil
	}
}

// WithAddedHeader adds a header parameter.
//
// Existing values are kept and the new value is added after them.
func WithAddedHeader(key, value string) FetchOption {
	return func(ctx *fetchContext) error {
		ctx.Request.Header.Add(key, value)
		return nil
	}
}

// WithHeader sets a header.
//
// Any existing values for the header are replaced.
func WithHeader(key, value string) FetchOption {
	return func(ctx *fetchContext) error {
		ctx.Request.Header.Set(key, value)
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
func WithBody(body io.Reader) FetchOption {
	return func(ctx *fetchContext) error {
		switch v := body.(type) {
		case *bytes.Buffer:
			ctx.Request.ContentLength = int64(v.Len())
		case *bytes.Reader:
			ctx.Request.ContentLength = int64(v.Len())
		case *strings.Reader:
			ctx.Request.ContentLength = int64(v.Len())
		}

		ctx.Request.Body = asReadCloser(body)
		return nil
	}
}

// WithBodyJSON encodes the given value as JSON and uses the result as the request body.
//
// If the Content-Type header is not set or empty, it will be set to "application/json".
func WithBodyJSON(v any, opts ...jsontext.Options) FetchOption {
	return func(ctx *fetchContext) error {
		body, err := json.Marshal(v, opts...)
		if err != nil {
			return err
		}

		if ctx.Request.Header.Get("Content-Type") == "" {
			ctx.Request.Header.Set("Content-Type", "application/json")
		}

		ctx.Request.ContentLength = int64(len(body))
		ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
		ctx.Request.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}

		return nil
	}
}

// Handler specifies methods for handling responses.
type Handler interface {
	// HandleResponse is called after receiving a response and is passed both the response and a pointer to the
	// value that should be filled with the response.
	HandleResponse(dst any, resp *http.Response) error
}

// HandlerFunc implements the [Handler] interface using itself as [Handler.HandleResponse] implementation.
type HandlerFunc func(dst any, resp *http.Response) error

// HandleResponse returns the result of calling h(dst, resp).
func (h HandlerFunc) HandleResponse(dst any, resp *http.Response) error {
	return h(dst, resp)
}

// ErrUnhandledResponse can be returned by [Handler.HandleResponse] when the implementation can not handle the
// given response.
var ErrUnhandledResponse = errors.New("github.com/nussjustin/httpc: unhandled response")

// WithHandler sets the [Handler] used by [Fetch] to process the response.
func WithHandler(h Handler) FetchOption {
	return func(ctx *fetchContext) error {
		ctx.Handler = h
		return nil
	}
}

// WithHandlerFunc is a shortcut for WithHandler(HandlerFunc(h)).
func WithHandlerFunc(h HandlerFunc) FetchOption {
	return WithHandler(h)
}

// HandlerChain wraps multiple [Handler] implementations in a single [Handler] that calls each underlying [Handler] in
// order of first to last, until one returns a nil error or any error that is not [ErrUnhandledResponse], as determined
// by [errors.Is].
//
// If the chain is empty or no [Handler] can handle the response, [ErrUnhandledResponse] is returned.
type HandlerChain []Handler

// HandleResponse implements the [Handler] interface.
func (h HandlerChain) HandleResponse(dst any, resp *http.Response) error {
	for i := range h {
		if err := h[i].HandleResponse(dst, resp); err == nil || !errors.Is(err, ErrUnhandledResponse) {
			return err
		}
	}

	return ErrUnhandledResponse
}

// ErrorHandler returns a [Handler] that returns the given error.
func ErrorHandler(err error) HandlerFunc {
	return func(any, *http.Response) error {
		return err
	}
}

// ConditionalHandler returns a [Handler] that calls the given handler only if cond returns true for the response.
func ConditionalHandler(cond func(*http.Response) bool, handler Handler) HandlerFunc {
	return func(dst any, resp *http.Response) error {
		if !cond(resp) {
			return ErrUnhandledResponse
		}

		return handler.HandleResponse(dst, resp)
	}
}

// ContentTypeHandler executes the given handler if the response content type matches the given content type.
//
// The handler will compare the response content type both as is as well as with any parameters removed. So a response
// content type like "application/json; charset=utf-8" will match against "application/json".
func ContentTypeHandler(contentType string, handler Handler) HandlerFunc {
	return ConditionalHandler(
		func(resp *http.Response) bool {
			value := resp.Header.Get("Content-Type")

			if value == contentType {
				return true
			}

			// Try to match without parameters
			value, _, _ = strings.Cut(value, ";")

			return value == contentType
		},
		handler,
	)
}

func discardBody(resp *http.Response) (err error) {
	defer func() {
		if cErr := resp.Body.Close(); cErr != nil && err == nil {
			err = cErr
		}
	}()

	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// DiscardBodyHandler returns a [Handler] that discards the response body and closes it, but otherwise does nothing.
func DiscardBodyHandler() HandlerFunc {
	return func(_ any, resp *http.Response) (err error) {
		return discardBody(resp)
	}
}

// ProblemHandler returns a [Handler] that detects JSON-encoded problem details as defined by RFC 9457.
//
// If the response returned a problem, it will be decoded and returned as error by [Fetch] and the response body will
// be closed.
func ProblemHandler() HandlerFunc {
	return ContentTypeHandler(
		problem.ContentType,
		HandlerFunc(func(_ any, resp *http.Response) (err error) {
			defer func() {
				if cErr := resp.Body.Close(); cErr != nil && err == nil {
					err = cErr
				}
			}()

			details, err := problem.From(resp)
			if err != nil {
				return err
			}

			return details
		}),
	)
}

// StatusHandler executes the given handler if the response status matches the given status.
func StatusHandler(statusCode int, handler Handler) HandlerFunc {
	return ConditionalHandler(
		func(resp *http.Response) bool {
			return resp.StatusCode == statusCode
		},
		handler,
	)
}

// UnmarshalJSONHandler returns a [Handler] that decodes the response body as JSON.
//
// The response body will automatically be closed.
func UnmarshalJSONHandler(opts ...jsontext.Options) HandlerFunc {
	return func(dst any, resp *http.Response) (err error) {
		defer func() {
			if cErr := resp.Body.Close(); cErr != nil && err == nil {
				err = cErr
			}
		}()

		return json.UnmarshalRead(resp.Body, dst, opts...)
	}
}

// UnmarshalXMLHandler returns a [Handler] that decodes the response body as JSON.
//
// The response body will automatically be closed.
func UnmarshalXMLHandler(strict bool) HandlerFunc {
	return func(dst any, resp *http.Response) (err error) {
		defer func() {
			if cErr := resp.Body.Close(); cErr != nil && err == nil {
				err = cErr
			}
		}()

		dec := xml.NewDecoder(resp.Body)
		dec.Strict = strict

		return dec.Decode(dst)
	}
}
