# httpc [![Go Reference](https://pkg.go.dev/badge/github.com/nussjustin/httpc.svg)](https://pkg.go.dev/github.com/nussjustin/httpc) [![Lint](https://github.com/nussjustin/httpc/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nussjustin/httpc/actions/workflows/golangci-lint.yml) [![Test](https://github.com/nussjustin/httpc/actions/workflows/test.yml/badge.svg)](https://github.com/nussjustin/httpc/actions/workflows/test.yml)

Package httpc provides functions for simplifying client-side HTTP request handling.

## Examples

### Request creation

The following creates a PATCH request to https://example.com/product/1234 with a JSON request body containing a product
name and accepting a JSON response.

```go
var apiURL, _ = url.Parse("https://example.com/")

func main() {
	ctx := context.Background()

	req, err := httpc.NewRequest(ctx, "PATCH", "/product/:productID",
		httpc.WithBaseURL(apiURL),
		httpc.WithHeader("Accept", "application/json"),
		httpc.WithPathValue("productID", "1234"),
		httpc.WithJSON(map[string]any{
			"name": "Jeans",
		}))
	if err != nil {
		panic(err)
	}

	// Normal net/http request handling
	resp, err := http.DefaultClient.Do(req)

	// ...
}
```

### Using Endpoint

Endpoints allow defining different options and logic for handling requests and responses to specific endpoints.

They also support automatically unmarshalling responses into Go types for example by reading the response JSON.

#### Defining an endpoint

An endpoint consists of a HTTP request method and URL as well as zero or more request options, as well as one or more
"Handlers", which are responsible for processing the response.

The following defines an endpoint for fetching products from an API endpoint returning a JSON body:

```go
var productEndpoint = &httpc.Endpoint[Product]{
    Method: "GET",
    URL: "https://example.com/product/:id",
    Options: []httpc.RequestOption{
        // Tell the server we want a JSON response
        httpc.WithHeader("Accept", "application/json"),
    },
    Handlers: []httpc.Handler{
        // Decode the response JSON, but only if the response code is 200
        httpc.StatusHandler(httpc.StatusOK, httpc.JSONHandler()),
    },
}
```

#### Making a request

Once created an endpoint can be used to make requests and return the unmarshalled response.

Using the endpoint from the previous example, this will fetch the product with ID 1234:

```go
product, _, err := productEndpoint.Do(context.Background(), httpc.WithPathValue("id", "1234"))
if err != nil {
	panic(err)
}
```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License
[MIT](https://choosealicense.com/licenses/mit/)