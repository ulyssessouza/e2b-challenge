package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	DatabaseURL       string
	RedisAddr         string
	HydraPublicURL    string
	OAuthClientID     string
	OAuthClientSecret string
	OAuthRedirectURI  string
	RateLimitPerMin   int
	// AuthRateLimitPerMin throttles unauthenticated /auth/* routes per IP.
	AuthRateLimitPerMin int
	// RateLimitFailOpen decides behavior when Redis is unreachable: true
	// allows all requests (availability over protection), false rejects
	// them with 503 (protection over availability).
	RateLimitFailOpen bool
	// MaxRunningSandboxesPerProject caps concurrently running sandboxes per
	// project (the "plan limits" part of the domain model). 0 disables it.
	MaxRunningSandboxesPerProject int
	// Database connection pool bounds (per process).
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetimeSecs int
}

func Load() *Config {
	return &Config{
		Port:                          getEnv("PORT", "8080"),
		DatabaseURL:                   getEnv("DATABASE_URL", "postgres://e2b:e2b@localhost:5432/e2b?sslmode=disable"),
		RedisAddr:                     getEnv("REDIS_ADDR", "localhost:6379"),
		HydraPublicURL:                getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		OAuthClientID:                 getEnv("OAUTH_CLIENT_ID", "e2b-assignment"),
		OAuthClientSecret:             getEnv("OAUTH_CLIENT_SECRET", "e2b-assignment-secret"),
		OAuthRedirectURI:              getEnv("OAUTH_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		RateLimitPerMin:               getIntEnv("RATE_LIMIT_PER_MIN", 300),
		AuthRateLimitPerMin:           getIntEnv("AUTH_RATE_LIMIT_PER_MIN", 60),
		RateLimitFailOpen:             getBoolEnv("RATE_LIMIT_FAIL_OPEN", true),
		MaxRunningSandboxesPerProject: getIntEnv("MAX_RUNNING_SANDBOXES_PER_PROJECT", 10),
		DBMaxOpenConns:                getIntEnv("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:                getIntEnv("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetimeSecs:         getIntEnv("DB_CONN_MAX_LIFETIME_SECS", 1800),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
