package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env           string
	Addr          string
	DatabaseURL   string
	DBMaxConns    int32
	DBMinConns    int32
	AdminEmail    string
	SessionTTL    time.Duration
	CookieSecure  bool
	TrustProxy    bool
	GlossarySync  bool
	SessionCookie string
}

func Load() Config {
	loadDotEnv(".env")
	env := strings.ToLower(envString("APP_ENV", "development"))
	return Config{
		Env:           env,
		Addr:          envString("ADDR", "127.0.0.1:8080"),
		DatabaseURL:   envString("DATABASE_URL", "postgres://polyphone:polyphone@127.0.0.1:55432/polyphone?sslmode=disable"),
		DBMaxConns:    int32(envInt("DB_MAX_CONNS", 12)),
		DBMinConns:    int32(envInt("DB_MIN_CONNS", 1)),
		AdminEmail:    strings.ToLower(strings.TrimSpace(envString("ADMIN_EMAIL", ""))),
		SessionTTL:    envDuration("SESSION_TTL", 30*24*time.Hour),
		CookieSecure:  envBool("COOKIE_SECURE", env == "production"),
		TrustProxy:    envBool("TRUST_PROXY", false),
		GlossarySync:  envBool("GLOSSARY_SYNC", false),
		SessionCookie: envString("SESSION_COOKIE", "pc_session"),
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
