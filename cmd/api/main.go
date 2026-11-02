package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YosuaChrialMartono/personal-be/internal/config"
	"github.com/YosuaChrialMartono/personal-be/internal/database"
	"github.com/YosuaChrialMartono/personal-be/internal/httpapi"
	"github.com/YosuaChrialMartono/personal-be/internal/wishlist"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	auth, err := httpapi.NewAuth(ctx, os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"), os.Getenv("OWNER_EMAIL"), os.Getenv("PUBLIC_ORIGIN"))
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	repository := wishlist.NewSQLiteRepository(db)
	server := &http.Server{
		Addr:              net.JoinHostPort(os.Getenv("LISTEN_HOST"), fmt.Sprint(cfg.Port)),
		Handler:           httpapi.NewRouter(db, wishlist.NewService(repository), logger, cfg.AllowedOrigins, auth),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", server.Addr)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("serve api: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown api: %w", err)
	}
	return nil
}
