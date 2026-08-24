package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"e2b-challenge/internal/config"
	"e2b-challenge/internal/jwks"
	"e2b-challenge/internal/observability"
	"e2b-challenge/internal/server"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup, err := observability.Setup(ctx, "e2b-sandbox-api")
	if err != nil {
		slog.Error("failed to setup observability", "error", err)
		os.Exit(1)
	}

	sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	if err := sqlDB.Ping(); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(cfg.DatabaseURL, "migrations"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	kf, err := jwks.NewProvider(ctx, cfg.HydraPublicURL+"/.well-known/jwks.json")
	if err != nil {
		slog.Error("failed to setup JWKS provider", "error", err)
		os.Exit(1)
	}

	e := server.New(cfg, sqlDB, rdb, kf)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		slog.Info("shutting down...")
		cleanup(ctx)
		sqlDB.Close()
		rdb.Close()
	}()

	slog.Info("starting server", "port", cfg.Port)
	e.Logger.Fatal(e.Start(":" + cfg.Port))
}

func runMigrations(dbURL, migrationsPath string) error {
	m, err := migrate.New("file://"+migrationsPath, dbURL)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}