package web

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"polyphone-checker/internal/store"
)

func TestRegisterCreatesAdminAndSession(t *testing.T) {
	data := newFakeStore()
	server := newTestServer(t, data, testConfig())
	handler := server.Handler()

	recorder := performRequest(t, handler, http.MethodPost, "/api/auth/register", mustJSON(t, map[string]any{
		"email": "Admin@Example.com", "password": "Password!123", "display_name": "管理员",
	}))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response userResponse
	decodeJSON(t, recorder, &response)
	if response.User == nil || response.User.Role != "admin" || response.User.Email != "admin@example.com" {
		t.Fatalf("registered user = %#v", response.User)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "pc_session" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookies = %#v", cookies)
	}

	item, ok := data.users["admin@example.com"]
	if !ok {
		t.Fatal("user was not persisted")
	}
	if !checkPassword(item.PasswordHash, "Password!123") {
		t.Fatal("password was not hashed correctly")
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/auth/me", "", cookies[0])
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.User == nil || response.User.Email != "admin@example.com" {
		t.Fatalf("me response = %d %#v", recorder.Code, response)
	}
}

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		code    int
		errCode string
	}{
		{name: "invalid email", body: `{"email":"bad","password":"Password!123","display_name":"x"}`, code: http.StatusBadRequest, errCode: "invalid_email"},
		{name: "short password", body: `{"email":"user@example.com","password":"short","display_name":"x"}`, code: http.StatusBadRequest, errCode: "invalid_password"},
		{name: "long password", body: mustJSON(t, map[string]any{"email": "user@example.com", "password": strings.Repeat("a", 73), "display_name": "x"}), code: http.StatusBadRequest, errCode: "invalid_password"},
		{name: "empty display name", body: `{"email":"user@example.com","password":"Password!123","display_name":"  "}`, code: http.StatusBadRequest, errCode: "invalid_display_name"},
		{name: "long display name", body: mustJSON(t, map[string]any{"email": "user@example.com", "password": "Password!123", "display_name": strings.Repeat("名", 41)}), code: http.StatusBadRequest, errCode: "invalid_display_name"},
		{name: "unknown field", body: `{"email":"user@example.com","password":"Password!123","display_name":"x","extra":1}`, code: http.StatusBadRequest, errCode: "bad_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t, newFakeStore(), testConfig())
			recorder := performRequest(t, server.Handler(), http.MethodPost, "/api/auth/register", test.body)
			var body errorResponse
			decodeJSON(t, recorder, &body)
			if recorder.Code != test.code || body.Code != test.errCode {
				t.Fatalf("status/code = %d %#v, want %d %s", recorder.Code, body, test.code, test.errCode)
			}
		})
	}

	t.Run("duplicate email", func(t *testing.T) {
		data := newFakeStore()
		data.addUser("user@example.com", "Password!123", "user")
		server := newTestServer(t, data, testConfig())
		recorder := performRequest(t, server.Handler(), http.MethodPost, "/api/auth/register", `{"email":"USER@example.com","password":"Password!123","display_name":"x"}`)
		var body errorResponse
		decodeJSON(t, recorder, &body)
		if recorder.Code != http.StatusConflict || body.Code != "email_taken" {
			t.Fatalf("duplicate = %d %#v", recorder.Code, body)
		}
	})

	t.Run("store failure", func(t *testing.T) {
		data := newFakeStore()
		data.createUserErr = errors.New("insert failed")
		server := newTestServer(t, data, testConfig())
		recorder := performRequest(t, server.Handler(), http.MethodPost, "/api/auth/register", `{"email":"user@example.com","password":"Password!123","display_name":"x"}`)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", recorder.Code)
		}
	})

	t.Run("session failure", func(t *testing.T) {
		data := newFakeStore()
		data.createSessionErr = errors.New("session insert failed")
		server := newTestServer(t, data, testConfig())
		recorder := performRequest(t, server.Handler(), http.MethodPost, "/api/auth/register", `{"email":"user@example.com","password":"Password!123","display_name":"x"}`)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", recorder.Code)
		}
	})
}

func TestLoginFlows(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		prepare  func(*fakeStore)
		want     int
		errCode  string
	}{
		{name: "success", email: "user@example.com", password: "Password!123", want: http.StatusOK},
		{name: "case insensitive email", email: "USER@example.com", password: "Password!123", want: http.StatusOK},
		{name: "wrong password", email: "user@example.com", password: "WrongPassword!123", want: http.StatusUnauthorized, errCode: "invalid_credentials"},
		{name: "unknown user", email: "missing@example.com", password: "Password!123", want: http.StatusUnauthorized, errCode: "invalid_credentials"},
		{name: "invalid email", email: "not-an-email", password: "Password!123", want: http.StatusUnauthorized, errCode: "invalid_credentials"},
		{name: "store failure", email: "user@example.com", password: "Password!123", prepare: func(data *fakeStore) {
			data.getUserErr = errors.New("query failed")
		}, want: http.StatusInternalServerError, errCode: "internal_error"},
		{name: "session failure", email: "user@example.com", password: "Password!123", prepare: func(data *fakeStore) {
			data.createSessionErr = errors.New("insert failed")
		}, want: http.StatusInternalServerError, errCode: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := newFakeStore()
			data.addUser("user@example.com", "Password!123", "user")
			if test.prepare != nil {
				test.prepare(data)
			}
			server := newTestServer(t, data, testConfig())
			recorder := performRequest(t, server.Handler(), http.MethodPost, "/api/auth/login", mustJSON(t, map[string]any{
				"email": test.email, "password": test.password,
			}))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
			if test.errCode != "" {
				var body errorResponse
				decodeJSON(t, recorder, &body)
				if body.Code != test.errCode {
					t.Fatalf("code = %q, want %q", body.Code, test.errCode)
				}
			} else if len(recorder.Result().Cookies()) != 1 {
				t.Fatal("successful login must set a session cookie")
			}
		})
	}
}

func TestMeAndLogout(t *testing.T) {
	data := newFakeStore()
	user := data.addUser("user@example.com", "Password!123", "user")
	cookie := data.addSession(user)
	server := newTestServer(t, data, testConfig())
	handler := server.Handler()

	recorder := performRequest(t, handler, http.MethodGet, "/api/auth/me", "")
	var response userResponse
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.User != nil {
		t.Fatalf("anonymous me = %d %#v", recorder.Code, response)
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/auth/me", "", cookie)
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.User == nil || response.User.ID != user.ID {
		t.Fatalf("authenticated me = %d %#v", recorder.Code, response)
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/auth/logout", "", cookie)
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout status = %d", recorder.Code)
	}
	cleared := recorder.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
		t.Fatalf("cleared cookie = %#v", cleared)
	}
	recorder = performRequest(t, handler, http.MethodGet, "/api/auth/me", "", cookie)
	decodeJSON(t, recorder, &response)
	if response.User != nil {
		t.Fatal("session should be deleted after logout")
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/auth/logout", "", &http.Cookie{Name: "pc_session", Value: "unknown"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("unknown-session logout = %d", recorder.Code)
	}
}

func TestAuthMiddlewareAndRoleGuard(t *testing.T) {
	data := newFakeStore()
	user := data.addUser("user@example.com", "Password!123", "user")
	moderator := data.addUser("moderator@example.com", "Password!123", "moderator")
	userCookie := data.addSession(user)
	moderatorCookie := data.addSession(moderator)
	server := newTestServer(t, data, testConfig())
	handler := server.Handler()

	if recorder := performRequest(t, handler, http.MethodGet, "/api/my/cases", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous auth = %d", recorder.Code)
	}
	if recorder := performRequest(t, handler, http.MethodGet, "/api/my/cases", "", &http.Cookie{Name: "pc_session", Value: "missing"}); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid session = %d", recorder.Code)
	}
	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases", "", userCookie); recorder.Code != http.StatusForbidden {
		t.Fatalf("user role guard = %d", recorder.Code)
	}
	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases", "", moderatorCookie); recorder.Code != http.StatusOK {
		t.Fatalf("moderator role guard = %d", recorder.Code)
	}
}

func TestAuthRateLimit(t *testing.T) {
	server := newTestServer(t, newFakeStore(), testConfig())
	server.authLimiter = newRateLimiter(60, 2)
	handler := server.Handler()
	body := `{"email":"bad","password":"Password!123","display_name":"x"}`
	for i := 0; i < 2; i++ {
		if recorder := performRequest(t, handler, http.MethodPost, "/api/auth/register", body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("request %d = %d", i+1, recorder.Code)
		}
	}
	recorder := performRequest(t, handler, http.MethodPost, "/api/auth/register", body)
	var response errorResponse
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusTooManyRequests || response.Code != "rate_limited" {
		t.Fatalf("rate limit = %d %#v", recorder.Code, response)
	}
}

func TestFakeUserPasswordHashesAreBcrypt(t *testing.T) {
	data := newFakeStore()
	data.addUser("test@example.com", "Password!123", "user")
	item := data.users["test@example.com"]
	if err := bcrypt.CompareHashAndPassword([]byte(item.PasswordHash), []byte("Password!123")); err != nil {
		t.Fatalf("fake user hash does not verify: %v", err)
	}
}

var _ = store.ErrNotFound
