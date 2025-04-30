package main

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProxyHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		targetURL  string
		wantStatus int
		wantHeader string
	}{
		{
			name:       "HTTP GET",
			method:     http.MethodGet,
			targetURL:  "http://httpbin.org/get",
			wantStatus: http.StatusOK,
			wantHeader: "GoCustomProxy",
		},
	}

	proxy := startTestProxy(t)
	defer proxy.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := http.Client{
				Transport: &http.Transport{
					Proxy: func(*http.Request) (*url.URL, error) {
						return url.Parse(proxy.URL)
					},
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
				Timeout: 5 * time.Second,
			}

			req, err := http.NewRequest(tt.method, tt.targetURL, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := client.Do(req)
			if err != nil && resp == nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}

			if tt.wantHeader != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), tt.wantHeader) {
					t.Errorf("Expected header %q in response body, but not found", tt.wantHeader)
				}
			}
		})
	}
}

func TestHandleHTTPProxy(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		body          string
		wantStatus    int
		wantBody      string
		expectError   bool
		modifyRequest func(*http.Request)
	}{
		{
			name: "Successful proxy",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("OK"))
			},
			wantStatus: http.StatusOK,
			wantBody:   "OK",
		},
		{
			name: "Upstream timeout",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * httpClient.Timeout)
			},
			wantStatus:  http.StatusBadGateway,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(tt.handler)
			defer ts.Close()

			req := httptest.NewRequest(http.MethodGet, ts.URL, nil)
			if tt.modifyRequest != nil {
				tt.modifyRequest(req)
			}

			w := httptest.NewRecorder()
			handleHTTPProxy(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}

			if tt.wantBody != "" {
				body, _ := io.ReadAll(resp.Body)
				if string(body) != tt.wantBody {
					t.Errorf("Expected body %q, got %q", tt.wantBody, string(body))
				}
			}
		})
	}
}

func TestHandleHTTPSProxy(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		expectError bool
	}{
		{"Valid host", "httpbin.org:443", false},
		{"Invalid port", "httpbin.org:abc", true},
		{"No port", "httpbin.org", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodConnect, "https://"+tt.host, nil)
			req.Host = tt.host

			handleHTTPSProxy(w, req)

			if tt.expectError && w.Code < 400 {
				t.Error("Expected error but got success")
			}
		})
	}
}

func TestIsClosedConnectionError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{io.EOF, true},
		{io.ErrClosedPipe, true},
		{net.ErrClosed, true},
		{errors.New("other error"), false},
	}

	for _, tt := range tests {
		result := isClosedConnectionError(tt.err)
		if result != tt.expected {
			t.Errorf("For error %v, expected %t got %t", tt.err, tt.expected, result)
		}
	}
}

func TestModifyRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	modifyRequest(req)

	if req.Header.Get("X-Proxy") != "GoCustomProxy" {
		t.Error("X-Proxy header not set")
	}
}

func TestCopyHeaders(t *testing.T) {
	src := http.Header{
		"Content-Type": []string{"text/plain"},
		"X-Test":       []string{"value"},
	}
	dst := http.Header{}

	copyHeaders(dst, src)

	if len(dst) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(dst))
	}
}

func startTestProxy(t *testing.T) *httptest.Server {
	proxy := httptest.NewServer(http.HandlerFunc(proxyHandler))
	t.Cleanup(func() {
		proxy.Close()
	})
	return proxy
}