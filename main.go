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
	"github.com/labstack/echo/v4"
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

	sqlDB := openDB(cfg)
	rdb := connectRedis(ctx, cfg)
	kf := connectJWKS(ctx, cfg)

	e := server.New(cfg, sqlDB, rdb, kf)
	runServer(e, cfg, func() {
		cancel()
		sqlDB.Close()
		rdb.Close()
	})
}

// openDB opens the connection pool, applies bounds, verifies connectivity and
// runs migrations. Any failure here is fatal: the service cannot start.
func openDB(cfg *config.Config) *sql.DB {
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
	return sqlDB
}

// connectRedis returns a client; an unreachable Redis degrades to disabled
// rate limiting (fail-open) rather than aborting startup.
func connectRedis(ctx context.Context, cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable, rate limiting disabled", "error", err)
	}
	return rdb
}

// connectJWKS retries briefly so a slow-starting Hydra does not leave auth
// hard-down until a manual restart. Once the attempts are spent it returns
// nil and protected routes fail closed (503) — see server.JWTAuth.
func connectJWKS(ctx context.Context, cfg *config.Config) keyfunc.Keyfunc {
	const attempts = 5
	var kf keyfunc.Keyfunc
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			slog.Warn("hydra unavailable, retrying", "attempt", attempt, "error", err)
			select {
			case <-ctx.Done():
				slog.Error("shutdown while waiting for hydra")
				os.Exit(1)
			case <-time.After(time.Duration(attempt-1) * time.Second):
			}
		}
		if kf, err = jwks.NewProvider(ctx, cfg.HydraPublicURL+"/.well-known/jwks.json"); err == nil {
			return kf
		}
	}
	slog.Warn("hydra unavailable, protected routes will fail closed", "error", err)
	return nil
}

// runServer starts the HTTP server and blocks until a signal arrives, then
// drains in-flight requests within a deadline and runs the cleanup.
func runServer(e *echo.Echo, cfg *config.Config, cleanup func()) {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown failed", "error", err)
	}

	cleanup()
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
