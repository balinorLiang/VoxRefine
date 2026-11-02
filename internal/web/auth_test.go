package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"polyphone-checker/internal/config"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password should verify")
	}
	if checkPassword(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != bcryptCost {
		t.Fatalf("bcrypt cost = %d, err=%v", cost, err)
	}
}

func TestSessionTokenIsRandomAndHashed(t *testing.T) {
	tokenA, hashA, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	tokenB, hashB, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if tokenA == tokenB || hashA == hashB {
		t.Fatal("two session tokens must not collide")
	}
	if len(tokenA) != 43 {
		t.Fatalf("token length = %d, want 43", len(tokenA))
	}
	if len(hashA) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hashA))
	}
	if got := hashSessionToken(tokenA); got != hashA {
		t.Fatalf("hashSessionToken = %q, want %q", got, hashA)
	}
	if strings.ContainsAny(tokenA, "+/=") {
		t.Fatalf("token is not URL-safe: %q", tokenA)
	}
}

func TestNormalizeEmailAndValidEmail(t *testing.T) {
	if got := normalizeEmail("  User@Example.COM  "); got != "user@example.com" {
		t.Fatalf("normalizeEmail = %q", got)
	}
	tests := []struct {
		email string
		want  bool
	}{
		{"user@example.com", true},
		{"first.last+tag@example.co.uk", true},
		{"", false},
		{"missing-at.example.com", false},
		{"display <user@example.com>", false},
		{"user @example.com", false},
		{"user@example.com\n", false},
		{strings.Repeat("a", 250) + "@example.com", false},
	}
	for _, test := range tests {
		if got := validEmail(test.email); got != test.want {
			t.Errorf("validEmail(%q) = %v, want %v", test.email, got, test.want)
		}
	}
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		query    string
		fallback int
		min      int
		max      int
		want     int
	}{
		{"", 50, 1, 100, 50},
		{"?limit=20", 50, 1, 100, 20},
		{"?limit=0", 50, 1, 100, 50},
		{"?limit=-2", 50, 1, 100, 50},
		{"?limit=999", 50, 1, 100, 100},
		{"?limit=abc", 50, 1, 100, 50},
		{"?limit=+25", 50, 1, 100, 25},
	}
	for _, test := range tests {
		req := httptest.NewRequest(http.MethodGet, "/test"+test.query, nil)
		if got := queryInt(req, "limit", test.fallback, test.min, test.max); got != test.want {
			t.Errorf("queryInt(%q) = %d, want %d", test.query, got, test.want)
		}
	}
}

func TestReadJSONValidation(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "valid", body: `{"name":"ok"}`},
		{name: "unknown field", body: `{"name":"ok","extra":true}`, wantErr: true},
		{name: "trailing object", body: `{"name":"ok"}{"name":"again"}`, wantErr: true},
		{name: "empty", body: ``, wantErr: true},
		{name: "invalid", body: `{"name":`, wantErr: true},
		{name: "too large", body: `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			var dst payload
			err := readJSON(recorder, req, &dst)
			if (err != nil) != test.wantErr {
				t.Fatalf("readJSON error = %v, wantErr=%v", err, test.wantErr)
			}
			if !test.wantErr && dst.Name != "ok" {
				t.Fatalf("payload = %#v", dst)
			}
		})
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	for _, secure := range []bool{false, true} {
		server := &Server{cfg: config.Config{SessionCookie: "pc_session", CookieSecure: secure}}
		recorder := httptest.NewRecorder()
		server.setSessionCookie(recorder, "token-value", expires)
		cookies := recorder.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies = %d", len(cookies))
		}
		cookie := cookies[0]
		if cookie.Name != "pc_session" || cookie.Value != "token-value" || cookie.Path != "/" {
			t.Fatalf("cookie = %#v", cookie)
		}
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Secure != secure {
			t.Fatalf("cookie flags = %#v", cookie)
		}
		if cookie.MaxAge <= 0 || cookie.Expires.IsZero() {
			t.Fatalf("cookie expiry = %#v", cookie)
		}
	}

	server := &Server{cfg: config.Config{SessionCookie: "pc_session"}}
	recorder := httptest.NewRecorder()
	server.clearSessionCookie(recorder)
	clear := recorder.Result().Cookies()[0]
	if clear.Name != "pc_session" || clear.Value != "" || clear.MaxAge >= 0 {
		t.Fatalf("clear cookie = %#v", clear)
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, test := range []struct {
		name     string
		env      string
		secure   bool
		wantHSTS bool
	}{
		{name: "development http", env: "development", secure: false, wantHSTS: false},
		{name: "production http", env: "production", secure: false, wantHSTS: false},
		{name: "production https", env: "production", secure: true, wantHSTS: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{cfg: config.Config{Env: test.env, CookieSecure: test.secure}}
			handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			headers := recorder.Header()
			for key, want := range map[string]string{
				"X-Content-Type-Options":       "nosniff",
				"X-Frame-Options":              "DENY",
				"Referrer-Policy":              "strict-origin-when-cross-origin",
				"Cross-Origin-Resource-Policy": "same-origin",
			} {
				if got := headers.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			for _, key := range []string{"Content-Security-Policy", "Permissions-Policy"} {
				if headers.Get(key) == "" {
					t.Errorf("%s is missing", key)
				}
			}
			if got := headers.Get("Strict-Transport-Security") != ""; got != test.wantHSTS {
				t.Errorf("HSTS present = %v, want %v", got, test.wantHSTS)
			}
		})
	}
}

func TestOriginGuard(t *testing.T) {
	server := &Server{}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := server.originGuard(next)

	tests := []struct {
		name   string
		method string
		path   string
		host   string
		header map[string]string
		want   int
	}{
		{name: "same origin write", method: http.MethodPost, path: "/api/check", host: "app.example", header: map[string]string{"Origin": "https://app.example"}, want: http.StatusNoContent},
		{name: "cross origin write", method: http.MethodPost, path: "/api/check", host: "app.example", header: map[string]string{"Origin": "https://evil.example"}, want: http.StatusForbidden},
		{name: "cross site fetch metadata", method: http.MethodPost, path: "/api/check", host: "app.example", header: map[string]string{"Sec-Fetch-Site": "cross-site"}, want: http.StatusForbidden},
		{name: "get ignores origin", method: http.MethodGet, path: "/api/check", host: "app.example", header: map[string]string{"Origin": "https://evil.example"}, want: http.StatusNoContent},
		{name: "non api write ignores origin", method: http.MethodPost, path: "/other", host: "app.example", header: map[string]string{"Origin": "https://evil.example"}, want: http.StatusNoContent},
		{name: "malformed origin", method: http.MethodPost, path: "/api/check", host: "app.example", header: map[string]string{"Origin": "://bad"}, want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://"+test.host+test.path, nil)
			for key, value := range test.header {
				req.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.want, recorder.Body.String())
			}
			if test.want == http.StatusForbidden {
				var body map[string]string
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if body["code"] != "bad_origin" {
					t.Fatalf("error code = %q", body["code"])
				}
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")

	direct := &Server{cfg: config.Config{TrustProxy: false}}
	if got := direct.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("direct IP = %q", got)
	}
	proxied := &Server{cfg: config.Config{TrustProxy: true}}
	if got := proxied.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("proxied IP = %q", got)
	}

	req.Header.Del("X-Forwarded-For")
	if got := proxied.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("fallback IP = %q", got)
	}
	req.RemoteAddr = "not-host-port"
	if got := proxied.clientIP(req); got != "not-host-port" {
		t.Fatalf("malformed remote addr = %q", got)
	}
}
