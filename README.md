# httpc [![Go Reference](https://pkg.go.dev/badge/github.com/nussjustin/httpc.svg)](https://pkg.go.dev/github.com/nussjustin/httpc) [![Lint](https://github.com/nussjustin/httpc/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nussjustin/httpc/actions/workflows/golangci-lint.yml) [![Test](https://github.com/nussjustin/httpc/actions/workflows/test.yml/badge.svg)](https://github.com/nussjustin/httpc/actions/workflows/test.yml)

Package httpc provides functions for simplifying client-side HTTP request handling.

## Examples

```go
product, err := httpc.Fetch[Product](ctx, "GET", "/product/:id",
    httpc.WithClient(client),
    httpc.WithBaseURL(baseURL),
    httpc.WithPathValue("id", "1234"))
```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License
[MIT](https://choosealicense.com/licenses/mit/)