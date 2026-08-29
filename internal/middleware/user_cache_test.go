package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"e2b-challenge/internal/db"
)

type countingResolver struct {
	calls int
	user  db.User
	err   error
}

func (c *countingResolver) GetUserByOAuthSub(_ context.Context, _ string) (db.User, error) {
	c.calls++
	return c.user, c.err
}

func TestCachedUserResolverHitsCacheWithinTTL(t *testing.T) {
	stub := &countingResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}
	cached := NewCachedUserResolver(stub, time.Minute, 100)

	for i := 0; i < 5; i++ {
		user, err := cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.ID != "u1" {
			t.Fatalf("unexpected user: %+v", user)
		}
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 underlying lookup, got %d", stub.calls)
	}
}

func TestCachedUserResolverRefetchesAfterTTL(t *testing.T) {
	stub := &countingResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}
	cached := NewCachedUserResolver(stub, 0, 100)

	_, _ = cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")
	_, _ = cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")

	if stub.calls != 2 {
		t.Fatalf("expected refetch with zero TTL, got %d calls", stub.calls)
	}
}

func TestCachedUserResolverPassesThroughErrors(t *testing.T) {
	stub := &countingResolver{err: errors.New("db down")}
	cached := NewCachedUserResolver(stub, time.Minute, 100)

	_, err := cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 underlying call, got %d", stub.calls)
	}
}

func TestCachedUserResolverDoesNotCacheErrors(t *testing.T) {
	stub := &countingResolver{user: db.User{ID: "u1"}, err: errors.New("db down")}
	cached := NewCachedUserResolver(stub, time.Minute, 100)

	_, _ = cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")
	stub.err = nil
	user, err := cached.GetUserByOAuthSub(context.Background(), "foo@bar.com")
	if err != nil {
		t.Fatalf("expected recovery after transient error: %v", err)
	}
	if user.ID != "u1" {
		t.Fatal("expected user from second call")
	}
}

func TestCachedUserResolverRespectsCapacity(t *testing.T) {
	stub := &countingResolver{user: db.User{ID: "u1"}}
	cached := NewCachedUserResolver(stub, time.Minute, 1)

	_, _ = cached.GetUserByOAuthSub(context.Background(), "a@x.com") // call 1, cached
	_, _ = cached.GetUserByOAuthSub(context.Background(), "b@x.com") // call 2, evicts a
	_, _ = cached.GetUserByOAuthSub(context.Background(), "a@x.com") // call 3, evicts b

	if stub.calls != 3 {
		t.Fatalf("expected 3 underlying calls with capacity 1, got %d", stub.calls)
	}
}

func TestCachedUserResolverKeepsWorkingAfterFillingUp(t *testing.T) {
	stub := &countingResolver{user: db.User{ID: "u1"}}
	cached := NewCachedUserResolver(stub, time.Minute, 2)

	// Fill beyond capacity with distinct users.
	for i := 0; i < 5; i++ {
		_, _ = cached.GetUserByOAuthSub(context.Background(), string(rune('a'+i))+"@x.com")
	}
	// A repeat of the most recent user must still be served from cache.
	_, _ = cached.GetUserByOAuthSub(context.Background(), "e@x.com")

	if stub.calls != 5 {
		t.Fatalf("expected recent entry to remain cached after eviction, got %d calls", stub.calls)
	}
}
