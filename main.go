package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"e2b-challenge/internal/config"
	"e2b-challenge/internal/jwks"
	"e2b-challenge/internal/server"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	// Without pool bounds, each instance opens unbounded Postgres
	// connections under load until the server-side limit is hit.
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DBConnMaxLifetimeSecs) * time.Second)
	if err := sqlDB.Ping(); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable, rate limiting disabled", "error", err)
	}

	var kf keyfunc.Keyfunc
	for attempt := 1; attempt <= 5; attempt++ {
		kf, err = jwks.NewProvider(ctx, cfg.HydraPublicURL+"/.well-known/jwks.json")
		if err == nil {
			break
		}
		if attempt == 5 {
			break
		}
		slog.Warn("hydra unavailable, retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			slog.Error("shutdown while waiting for hydra")
			os.Exit(1)
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	if kf == nil {
		slog.Warn("hydra unavailable, authentication disabled", "error", err)
	}

	e := server.New(cfg, sqlDB, rdb, kf)

	// Timeouts protect the server from slow clients (slowloris-style
	// connection exhaustion); echo.Start alone would use zero timeouts.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "port", cfg.Port)
		if err := e.StartServer(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			quit <- syscall.SIGTERM
		}
	}()

	<-quit
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown failed", "error", err)
	}

	cancel()
	sqlDB.Close()
	rdb.Close()
	slog.Info("shutdown complete")
}

func runMigrations(dbURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}
