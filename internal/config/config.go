package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	RedisAddr              string
	HydraPublicURL         string
	OAuthClientID          string
	OAuthClientSecret      string
	OAuthRedirectURI       string
	RateLimitPerMin        int
	RateLimitSandboxPerMin int
}

func Load() *Config {
	return &Config{
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://e2b:e2b@localhost:5432/e2b?sslmode=disable"),
		RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
		HydraPublicURL:         getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		OAuthClientID:          getEnv("OAUTH_CLIENT_ID", "e2b-assignment"),
		OAuthClientSecret:      getEnv("OAUTH_CLIENT_SECRET", "e2b-assignment-secret"),
		OAuthRedirectURI:       getEnv("OAUTH_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		RateLimitPerMin:        getIntEnv("RATE_LIMIT_PER_MIN", 1000),
		RateLimitSandboxPerMin: getIntEnv("RATE_LIMIT_SANDBOX_PER_MIN", 100),
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