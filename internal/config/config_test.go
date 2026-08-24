package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("REDIS_ADDR", "localhost:6380")
	t.Setenv("HYDRA_PUBLIC_URL", "http://localhost:4444")
	t.Setenv("OAUTH_CLIENT_ID", "test-client")
	t.Setenv("OAUTH_CLIENT_SECRET", "test-secret")
	t.Setenv("OAUTH_REDIRECT_URI", "http://localhost:9090/auth/callback")
	t.Setenv("RATE_LIMIT_PER_MIN", "500")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://localhost:5432/test" {
		t.Errorf("expected DatabaseURL postgres://localhost:5432/test, got %s", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "localhost:6380" {
		t.Errorf("expected RedisAddr localhost:6380, got %s", cfg.RedisAddr)
	}
	if cfg.HydraPublicURL != "http://localhost:4444" {
		t.Errorf("expected HydraPublicURL http://localhost:4444, got %s", cfg.HydraPublicURL)
	}
	if cfg.OAuthClientID != "test-client" {
		t.Errorf("expected OAuthClientID test-client, got %s", cfg.OAuthClientID)
	}
	if cfg.OAuthClientSecret != "test-secret" {
		t.Errorf("expected OAuthClientSecret test-secret, got %s", cfg.OAuthClientSecret)
	}
	if cfg.OAuthRedirectURI != "http://localhost:9090/auth/callback" {
		t.Errorf("expected OAuthRedirectURI http://localhost:9090/auth/callback, got %s", cfg.OAuthRedirectURI)
	}
	if cfg.RateLimitPerMin != 500 {
		t.Errorf("expected RateLimitPerMin 500, got %d", cfg.RateLimitPerMin)
	}
}