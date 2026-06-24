package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api-monitor/internal/api"
	"api-monitor/internal/auth"
	"api-monitor/internal/cache"
	"api-monitor/internal/config"
	"api-monitor/internal/connectors"
	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/db"
	"api-monitor/internal/notify"
	"api-monitor/internal/scanner"
	"api-monitor/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api-monitor failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	command := "api"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, cfg.MigrationsDir); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if command == "migrate" {
		slog.Info("migrations applied")
		return nil
	}

	cacheSvc := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err := cacheSvc.Ping(ctx); err != nil {
		slog.Warn("redis ping failed; continuing without cache guarantees", "error", err)
	}
	defer cacheSvc.Close()

	secretSvc := appcrypto.New(cfg.AppSecret)
	st := store.New(pool, secretSvc)
	authSvc := auth.New(cfg.AppSecret, cfg.JWTIssuer, cfg.JWTTTL)
	httpClient := &http.Client{Timeout: 25 * time.Second}
	registry := connectors.NewRegistry(httpClient)
	notifier := notify.New(httpClient)
	scannerSvc := scanner.New(st, registry, notifier, cacheSvc, slog.Default())

	switch command {
	case "api":
		return runAPI(ctx, cfg, st, authSvc, scannerSvc, cacheSvc, notifier)
	case "worker":
		slog.Info("worker started")
		err := scannerSvc.RunLoop(ctx, 15*time.Second)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unknown command %q; expected api, worker, or migrate", command)
	}
}

func runAPI(ctx context.Context, cfg config.Config, st *store.Store, authSvc auth.Service, scannerSvc *scanner.Service, cacheSvc *cache.Cache, notifierSvc *notify.Service) error {
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(st, authSvc, scannerSvc, cacheSvc, notifierSvc, cfg, &http.Client{Timeout: 10 * time.Second}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
