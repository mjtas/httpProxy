package main

import (
"errors"
"io"
"log"
"net"
"net/http"
"net/url"
"time"
)

// Reusable HTTP client with connection pooling and timeouts
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	Timeout: 10 * time.Second,
}

// modifyRequest modifies incoming requests before forwarding
func modifyRequest(req *http.Request) {
	req.Header.Set("X-Proxy", "GoCustomProxy")
	log.Printf("Proxying request: %s %s", req.Method, req.URL)
}

// copyHeaders copies headers from source to destination
func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Set(key, value) //
		}
	}
}

// handleHTTPProxy handles HTTP traffic
func handleHTTPProxy(w http.ResponseWriter, req *http.Request) {
	modifyRequest(req)

	// Create a new client request
	targetURL := &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}

	if targetURL.Scheme == "" {
		targetURL.Scheme = "http" // Default to HTTP
	}
	if targetURL.Host == "" {
		targetURL.Host = req.Host // Fallback to original host
	}

	newReq, err := http.NewRequest(req.Method, targetURL.String(), req.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	copyHeaders(newReq.Header, req.Header)

	resp, err := httpClient.Do(newReq)
	if err != nil {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Gateway error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response body: %v", err)
	}
}

// handleHTTPSProxy handles HTTPS traffic using CONNECT tunnel
func handleHTTPSProxy(w http.ResponseWriter, req *http.Request) {
	// Validate host format
	host := req.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443") // Default to HTTPS port
	}

	destConn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		log.Printf("Dial error: %v", err)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("Hijack error: %v", err)
		http.Error(w, "Connection failed", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Bidirectional data copy with graceful shutdown
	errCh := make(chan error, 2)
	go func() { errCh <- copyStream(destConn, clientConn) }()
	go func() { errCh <- copyStream(clientConn, destConn) }()

	// Wait for both directions to complete
	<-errCh
	<-errCh
}

// copyStream handles the actual data transfer with logging
func copyStream(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if err != nil && !isClosedConnectionError(err) {
		log.Printf("Copy error: %v", err)
	}
	return err
}

// isClosedConnectionError checks for expected connection closure errors
func isClosedConnectionError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return err == io.EOF ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed)
}

// proxyHandler routes requests to appropriate handler
func proxyHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		handleHTTPSProxy(w, req)
	} else {
		handleHTTPProxy(w, req)
	}
}

func main() {
	server := &http.Server{
		Addr:           ":8080",
		Handler:        http.HandlerFunc(proxyHandler),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	log.Printf("Starting proxy server on %s...", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}