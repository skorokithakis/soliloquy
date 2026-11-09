package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/config"
	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := database.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if err := ensureFirstAdminInvite(context.Background(), database, cfg); err != nil {
		return fmt.Errorf("create first admin invite: %w", err)
	}

	server, err := web.NewServer(database, cfg)
	if err != nil {
		return fmt.Errorf("create web server: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Cancel request contexts on shutdown so long-lived SSE connections
		// return instead of waiting for the shutdown timeout.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		if cfg.AllowAnyOrigin {
			slog.Warn("Origin check is off: CSRF protection is disabled. Use this for QA only.")
		}
		slog.Info("listening", "addr", cfg.ListenAddr, "db", cfg.DBPath, "base_url", cfg.BaseURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// ensureFirstAdminInvite creates a one-time admin invite when the database has
// no users and prints its URL to the log. The URL is the only way to create the
// first account.
func ensureFirstAdminInvite(ctx context.Context, database *db.DB, cfg config.Config) error {
	token, err := auth.GenerateToken()
	if err != nil {
		return err
	}

	created, err := database.EnsureBootstrapInvite(ctx, token, time.Now())
	if err != nil {
		return err
	}
	if created {
		slog.Info("first admin invite", "url", cfg.BaseURL+"/invite/"+token)
	}
	return nil
}
