# httpProxy
A lightweight HTTP/HTTPS roxy server written in Go. This proxy supports both standard HTTP requests and HTTPS tunneling via the CONNECT method, with proper connection pooling, timeout handling, and header management.

## Features
- Full support for HTTP and HTTPS proxying
- Connection pooling via http.Transport
- Timeouts to avoid hanging connections
- Request/response header copying and customisation
- Graceful handling of common network errors
- Minimal logging for observability

## How to use
- Install Go 1.16 or later
- Run the commands:
```go build -o gocustomproxy```
```./gocustomproxy```

## How it works
#### HTTP proxying
- The request is modified (custom header is added)
- The request is forwarded to the target server
- The response is read and sent back to the client

#### HTTPS proxying (CONNECT tunnelling)
- A TCP connection is established to the requested host
- The client connection is hijacked to enable raw TCP transfer
- Bidirectional copying is set up between the client and destination

## Logging
The proxy logs all proxied HTTP requests (method and URL), errors during request forwarding or streaming and connection issues

## Security Considerations
This is a basic proxy without authentication, rate limiting, or TLS termination. Do not expose it to the open internet without additional safeguards.