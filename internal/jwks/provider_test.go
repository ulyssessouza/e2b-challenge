package jwks

import (
	"context"
	"testing"
	"time"
)

func TestNewProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kf, err := NewProvider(ctx, "http://localhost:4444/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	if kf == nil {
		t.Fatal("expected non-nil keyfunc")
	}
}