package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	glossarydata "polyphone-checker/data"
	"polyphone-checker/internal/config"
	"polyphone-checker/internal/database"
	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
	"polyphone-checker/internal/web"
	"polyphone-checker/migrations"
)

const version = "2.0.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	st := store.New(pool)
	seed, err := glossary.LoadSeed(glossarydata.FS)
	if err != nil {
		return err
	}
	seeded, err := st.SeedGlossary(ctx, seed, cfg.GlossarySync)
	if err != nil {
		return err
	}
	if seeded > 0 {
		slog.Info("glossary seeded", "version", seed.Version, "terms", seeded)
	}

	entries, err := st.LoadGlossary(ctx)
	if err != nil {
		return err
	}
	detector := glossary.NewDetector(entries)
	for _, warning := range detector.Warnings() {
		slog.Warn("glossary warning", "detail", warning)
	}

	app := web.New(cfg, st, detector, version)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	cleanupCtx, cancelCleanup := context.WithCancel(ctx)
	defer cancelCleanup()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				if err := st.DeleteExpiredSessions(cleanupCtx); err != nil {
					slog.Warn("session cleanup failed", "error", err)
				}
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "terms", len(entries), "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
