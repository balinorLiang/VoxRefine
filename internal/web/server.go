package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"polyphone-checker/internal/config"
	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
	liteassets "polyphone-checker/lite"
	webassets "polyphone-checker/web"
)

type Server struct {
	cfg          config.Config
	store        DataStore
	detector     *glossary.Detector
	mux          *http.ServeMux
	limiter      *rateLimiter
	authLimiter  *rateLimiter
	checkLimiter *rateLimiter
	writeLimiter *rateLimiter
	dummyHash    []byte
	version      string
	logger       *slog.Logger
}

type authedHandler func(http.ResponseWriter, *http.Request, store.User)

// DataStore is the persistence surface used by the HTTP layer. Keeping it as
// an interface makes handlers testable without a live PostgreSQL instance.
type DataStore interface {
	Ping(ctx context.Context) error
	SearchTerms(ctx context.Context, query, termType, risk string, limit, offset int) ([]glossary.Entry, int, error)
	GetTerm(ctx context.Context, id string) (glossary.Entry, error)
	Stats(ctx context.Context) (store.Stats, error)
	CreateUser(ctx context.Context, email, passwordHash, displayName, role string) (store.User, error)
	GetUserByEmail(ctx context.Context, email string) (store.User, string, error)
	CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	UserBySession(ctx context.Context, tokenHash string) (store.User, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	ListCases(ctx context.Context, status, viewerID, filterUserID string, limit, offset int) ([]store.Case, error)
	CreateCase(ctx context.Context, userID, termID, wrongText, correctPinyin, note string) (string, error)
	VoteCase(ctx context.Context, caseID, userID string) (int, error)
	UnvoteCase(ctx context.Context, caseID, userID string) (int, error)
	ModerateCase(ctx context.Context, caseID, moderatorID, status, note string) error
}

type contextKey string

const userContextKey contextKey = "user"

func New(cfg config.Config, st DataStore, detector *glossary.Detector, version string) *Server {
	s := &Server{
		cfg:          cfg,
		store:        st,
		detector:     detector,
		mux:          http.NewServeMux(),
		limiter:      newRateLimiter(120, 40),
		authLimiter:  newRateLimiter(12, 6),
		checkLimiter: newRateLimiter(60, 20),
		writeLimiter: newRateLimiter(20, 10),
		dummyHash:    newDummyHash(),
		version:      version,
		logger:       slog.Default(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux
	handler = s.originGuard(handler)
	handler = s.rateLimit(handler)
	handler = s.securityHeaders(handler)
	handler = s.recoverer(handler)
	handler = s.requestLogger(handler)
	return handler
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/check", s.handleCheck)
	s.mux.HandleFunc("GET /api/terms", s.handleTerms)
	s.mux.HandleFunc("GET /api/terms/{id}", s.handleTerm)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)

	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", s.handleMe)

	s.mux.HandleFunc("GET /api/cases", s.handleCases)
	s.mux.HandleFunc("POST /api/cases", s.auth(s.handleCreateCase))
	s.mux.HandleFunc("GET /api/my/cases", s.auth(s.handleMyCases))
	s.mux.HandleFunc("POST /api/cases/{id}/vote", s.auth(s.handleVote))
	s.mux.HandleFunc("DELETE /api/cases/{id}/vote", s.auth(s.handleUnvote))

	s.mux.HandleFunc("GET /api/admin/cases", s.requireRole("moderator", "admin")(s.handleAdminCases))
	s.mux.HandleFunc("POST /api/admin/cases/{id}/moderate", s.requireRole("moderator", "admin")(s.handleModerate))

	webFS, err := fs.Sub(webassets.FS, ".")
	if err != nil {
		panic(err)
	}
	liteFS, err := fs.Sub(liteassets.FS, ".")
	if err != nil {
		panic(err)
	}
	var webHandler http.Handler = http.FileServer(http.FS(webFS))
	var liteHandler http.Handler = http.FileServer(http.FS(liteFS))
	if s.cfg.Env == "development" {
		if _, statErr := os.Stat("web/index.html"); statErr == nil {
			webHandler = http.FileServer(http.Dir("web"))
		}
		if _, statErr := os.Stat("lite/index.html"); statErr == nil {
			liteHandler = http.FileServer(http.Dir("lite"))
		}
	}
	s.mux.Handle("GET /lite/", s.staticCache(http.StripPrefix("/lite/", liteHandler)))
	s.mux.HandleFunc("GET /lite", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/lite/", http.StatusMovedPermanently)
	})
	s.mux.Handle("GET /", s.staticCache(webHandler))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	status := "ok"
	code := http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":   status,
		"database": status,
		"version":  s.version,
	})
}

func (s *Server) auth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.currentUser(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, "请先登录", "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx), user)
	}
}

func (s *Server) requireRole(roles ...string) func(authedHandler) http.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next authedHandler) http.HandlerFunc {
		return s.auth(func(w http.ResponseWriter, r *http.Request, user store.User) {
			if !allowed[user.Role] {
				apiError(w, http.StatusForbidden, "没有权限执行此操作", "forbidden")
				return
			}
			next(w, r, user)
		})
	}
}

func (s *Server) currentUser(r *http.Request) (store.User, bool) {
	cookie, err := r.Cookie(s.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return store.User{}, false
	}
	user, err := s.store.UserBySession(r.Context(), hashSessionToken(cookie.Value))
	if err != nil {
		return store.User{}, false
	}
	return user, true
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", s.clientIP(r),
		)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic", "error", fmt.Sprint(recovered), "stack", string(debug.Stack()))
				if strings.HasPrefix(r.URL.Path, "/api/") {
					apiError(w, http.StatusInternalServerError, "服务器内部错误", "internal_error")
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if s.cfg.Env == "production" && s.cfg.CookieSecure {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
				apiError(w, http.StatusForbidden, "跨站请求被拒绝", "bad_origin")
				return
			}
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			apiError(w, http.StatusForbidden, "跨站请求被拒绝", "bad_origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !s.limiter.allow(s.clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				apiError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试", "rate_limited")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || strings.HasSuffix(path, ".html") || path == "/lite/" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func apiError(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

func queryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}
