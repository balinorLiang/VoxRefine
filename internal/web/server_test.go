package web

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	data := newFakeStore()
	server := newTestServer(t, data, testConfig())

	recorder := performRequest(t, server.Handler(), http.MethodGet, "/api/health", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Version  string `json:"version"`
	}
	decodeJSON(t, recorder, &response)
	if response.Status != "ok" || response.Database != "ok" || response.Version != "test-version" {
		t.Fatalf("health response = %#v", response)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("security headers are missing from health response")
	}

	data.pingErr = errors.New("database offline")
	recorder = performRequest(t, server.Handler(), http.MethodGet, "/api/health", "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded status = %d", recorder.Code)
	}
	decodeJSON(t, recorder, &response)
	if response.Status != "degraded" || response.Database != "degraded" {
		t.Fatalf("degraded response = %#v", response)
	}
}

func TestStaticRoutes(t *testing.T) {
	server := newTestServer(t, newFakeStore(), testConfig())
	handler := server.Handler()

	tests := []struct {
		path        string
		wantStatus  int
		wantContent string
		wantCache   string
	}{
		{path: "/", wantStatus: http.StatusOK, wantContent: "多音字检查器", wantCache: "no-cache"},
		{path: "/lite/", wantStatus: http.StatusOK, wantContent: "多音字检查器", wantCache: "no-cache"},
		{path: "/app.js", wantStatus: http.StatusOK, wantContent: "function", wantCache: "public, max-age=3600"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := performRequest(t, handler, http.MethodGet, test.path, "")
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
			if test.wantContent != "" && !strings.Contains(recorder.Body.String(), test.wantContent) {
				t.Fatalf("body does not contain %q", test.wantContent)
			}
			if got := recorder.Header().Get("Cache-Control"); got != test.wantCache {
				t.Fatalf("Cache-Control = %q, want %q", got, test.wantCache)
			}
		})
	}

	recorder := performRequest(t, handler, http.MethodGet, "/lite", "")
	if recorder.Code != http.StatusMovedPermanently || recorder.Header().Get("Location") != "/lite/" {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestRecovererReturnsInternalError(t *testing.T) {
	server := newTestServer(t, newFakeStore(), testConfig())
	server.mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	recorder := performRequest(t, server.Handler(), http.MethodGet, "/panic", "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers were lost after panic")
	}
}

func TestAPIRateLimit(t *testing.T) {
	server := newTestServer(t, newFakeStore(), testConfig())
	server.limiter = newRateLimiter(60, 2)
	handler := server.Handler()

	for i := 0; i < 2; i++ {
		if recorder := performRequest(t, handler, http.MethodGet, "/api/stats", ""); recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d", i+1, recorder.Code)
		}
	}
	recorder := performRequest(t, handler, http.MethodGet, "/api/stats", "")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limited status = %d", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", recorder.Header().Get("Retry-After"))
	}
}
