package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"e2b-challenge/internal/config"
	"e2b-challenge/internal/jwks"
	"e2b-challenge/internal/server"
)

func main() {
	cfg := config.Load()

	sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	if err := runMigrations(cfg.DatabaseURL, "migrations"); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}

	kf, err := jwks.NewProvider(context.Background(), cfg.HydraPublicURL+"/.well-known/jwks.json")
	if err != nil {
		log.Fatalf("failed to setup JWKS provider: %v", err)
	}

	e := server.New(cfg, sqlDB, rdb, kf)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		log.Println("shutting down...")
		sqlDB.Close()
		rdb.Close()
	}()

	log.Printf("starting server on :%s", cfg.Port)
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