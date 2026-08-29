package jwks

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewProvider(t *testing.T) {
	// Skip instead of failing when the compose stack isn't running.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:4444/.well-known/jwks.json", nil)
	if err != nil {
		t.Fatalf("building probe request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skip("hydra not available — integration test")
	}
	resp.Body.Close()

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kf, err := NewProvider(ctx, "http://localhost:4444/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	if kf == nil {
		t.Fatal("expected non-nil keyfunc")
	}
}
