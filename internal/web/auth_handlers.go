package web

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"polyphone-checker/internal/store"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("register:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "注册请求过于频繁，请稍后再试", "rate_limited")
		return
	}
	var request registerRequest
	if err := readJSON(w, r, &request); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式不正确", "bad_request")
		return
	}
	email := normalizeEmail(request.Email)
	if !validEmail(email) {
		apiError(w, http.StatusBadRequest, "请输入有效的邮箱地址", "invalid_email")
		return
	}
	if len([]byte(request.Password)) < 8 || len([]byte(request.Password)) > 72 {
		apiError(w, http.StatusBadRequest, "密码长度需要在 8 到 72 个字节之间", "invalid_password")
		return
	}
	displayName := strings.TrimSpace(request.DisplayName)
	if utf8.RuneCountInString(displayName) < 1 || utf8.RuneCountInString(displayName) > 40 {
		apiError(w, http.StatusBadRequest, "昵称长度需要在 1 到 40 个字之间", "invalid_display_name")
		return
	}

	role := "user"
	if s.cfg.AdminEmail != "" && strings.EqualFold(email, s.cfg.AdminEmail) {
		role = "admin"
	}
	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "注册失败，请稍后再试", "internal_error")
		return
	}
	user, err := s.store.CreateUser(r.Context(), email, passwordHash, displayName, role)
	if errors.Is(err, store.ErrEmailTaken) {
		apiError(w, http.StatusConflict, "这个邮箱已经注册过了", "email_taken")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "注册失败，请稍后再试", "internal_error")
		return
	}
	if err := s.startSession(w, r, user); err != nil {
		apiError(w, http.StatusInternalServerError, "登录状态创建失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authLimiter.allow("login:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试", "rate_limited")
		return
	}
	var request loginRequest
	if err := readJSON(w, r, &request); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式不正确", "bad_request")
		return
	}
	email := normalizeEmail(request.Email)
	if !validEmail(email) || request.Password == "" {
		apiError(w, http.StatusUnauthorized, "邮箱或密码错误", "invalid_credentials")
		return
	}
	user, passwordHash, err := s.store.GetUserByEmail(r.Context(), email)
	if errors.Is(err, store.ErrNotFound) {
		_ = checkPassword(string(s.dummyHash), request.Password)
		apiError(w, http.StatusUnauthorized, "邮箱或密码错误", "invalid_credentials")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "登录失败，请稍后再试", "internal_error")
		return
	}
	if !checkPassword(passwordHash, request.Password) {
		apiError(w, http.StatusUnauthorized, "邮箱或密码错误", "invalid_credentials")
		return
	}
	if err := s.startSession(w, r, user); err != nil {
		apiError(w, http.StatusInternalServerError, "登录状态创建失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(s.cfg.SessionCookie); err == nil && cookie.Value != "" {
		_ = s.store.DeleteSession(r.Context(), hashSessionToken(cookie.Value))
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(r.Context(), user.ID, tokenHash, expires); err != nil {
		return err
	}
	s.setSessionCookie(w, token, expires)
	return nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validEmail(value string) bool {
	if value == "" || len(value) > 254 || !strings.Contains(value, "@") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}
