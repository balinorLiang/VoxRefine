package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var configKeys = []string{
	"APP_ENV",
	"ADDR",
	"DATABASE_URL",
	"DB_MAX_CONNS",
	"DB_MIN_CONNS",
	"ADMIN_EMAIL",
	"SESSION_TTL",
	"SESSION_COOKIE",
	"COOKIE_SECURE",
	"TRUST_PROXY",
	"GLOSSARY_SYNC",
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		key := key
		old, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, old)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, configKeys...)

	cfg := Load()

	if cfg.Env != "development" {
		t.Fatalf("Env = %q, want development", cfg.Env)
	}
	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL must have a development default")
	}
	if cfg.DBMaxConns != 12 || cfg.DBMinConns != 1 {
		t.Fatalf("connections = %d/%d, want 12/1", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.AdminEmail != "" {
		t.Fatalf("AdminEmail = %q, want empty", cfg.AdminEmail)
	}
	if cfg.SessionTTL != 30*24*time.Hour {
		t.Fatalf("SessionTTL = %v", cfg.SessionTTL)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure must default to false in development")
	}
	if cfg.TrustProxy || cfg.GlossarySync {
		t.Fatalf("boolean defaults = trust:%v sync:%v", cfg.TrustProxy, cfg.GlossarySync)
	}
	if cfg.SessionCookie != "pc_session" {
		t.Fatalf("SessionCookie = %q", cfg.SessionCookie)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, configKeys...)
	t.Setenv("APP_ENV", "PRODUCTION")
	t.Setenv("ADDR", "0.0.0.0:9090")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("DB_MAX_CONNS", "24")
	t.Setenv("DB_MIN_CONNS", "4")
	t.Setenv("ADMIN_EMAIL", "  Admin@Example.COM  ")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("SESSION_COOKIE", "custom_session")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("TRUST_PROXY", "true")
	t.Setenv("GLOSSARY_SYNC", "true")

	cfg := Load()

	if cfg.Env != "production" {
		t.Fatalf("Env = %q, want production", cfg.Env)
	}
	if cfg.Addr != "0.0.0.0:9090" || cfg.DatabaseURL != "postgres://example" {
		t.Fatalf("unexpected address/database: %q %q", cfg.Addr, cfg.DatabaseURL)
	}
	if cfg.DBMaxConns != 24 || cfg.DBMinConns != 4 {
		t.Fatalf("connections = %d/%d", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.AdminEmail != "admin@example.com" {
		t.Fatalf("AdminEmail = %q", cfg.AdminEmail)
	}
	if cfg.SessionTTL != 2*time.Hour || cfg.SessionCookie != "custom_session" {
		t.Fatalf("session = %v %q", cfg.SessionTTL, cfg.SessionCookie)
	}
	if cfg.CookieSecure {
		t.Fatal("explicit COOKIE_SECURE=false must override production default")
	}
	if !cfg.TrustProxy || !cfg.GlossarySync {
		t.Fatalf("booleans = trust:%v sync:%v", cfg.TrustProxy, cfg.GlossarySync)
	}
}

func TestLoadProductionDefaultsToSecureCookie(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, configKeys...)
	t.Setenv("APP_ENV", "production")

	if cfg := Load(); !cfg.CookieSecure {
		t.Fatal("production must default CookieSecure to true")
	}
}

func TestLoadInvalidValuesFallBack(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, configKeys...)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_MAX_CONNS", "not-a-number")
	t.Setenv("DB_MIN_CONNS", "0")
	t.Setenv("SESSION_TTL", "tomorrow")
	t.Setenv("TRUST_PROXY", "sometimes")
	t.Setenv("GLOSSARY_SYNC", "sometimes")

	cfg := Load()

	if cfg.DBMaxConns != 12 || cfg.DBMinConns != 1 {
		t.Fatalf("connections = %d/%d", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.SessionTTL != 30*24*time.Hour {
		t.Fatalf("SessionTTL = %v", cfg.SessionTTL)
	}
	if cfg.TrustProxy || cfg.GlossarySync {
		t.Fatalf("booleans = trust:%v sync:%v", cfg.TrustProxy, cfg.GlossarySync)
	}
	if !cfg.CookieSecure {
		t.Fatal("production fallback must keep CookieSecure=true")
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n" +
		"export FOO=from_file\n" +
		"BAR=\"quoted value\"\n" +
		"BAZ='single value'\n" +
		"EQUALS=left=right\n" +
		"EMPTY=\n" +
		"EXISTING=from_file\n" +
		"DUP=first\n" +
		"DUP=second\n" +
		"INVALID_LINE\n" +
		"=missing-key\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	unsetEnv(t, "FOO", "BAR", "BAZ", "EQUALS", "EMPTY", "DUP")
	t.Setenv("EXISTING", "from_process")
	loadDotEnv(path)

	for key, want := range map[string]string{
		"BAR":    "quoted value",
		"BAZ":    "single value",
		"EQUALS": "left=right",
		"EMPTY":  "",
		"DUP":    "first",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := os.Getenv("EXISTING"); got != "from_process" {
		t.Fatalf("dotenv must not override process env, got %q", got)
	}
}

func TestLoadDotEnvMissingFileIsNoop(t *testing.T) {
	loadDotEnv(filepath.Join(t.TempDir(), "missing.env"))
}
