package middleware

import (
	"context"
	"sync"
	"time"

	"e2b-challenge/internal/db"
)

// CachedUserResolver decorates a UserResolver with a TTL cache. JWT
// authentication otherwise pays a database round-trip on every request just to
// resolve the token subject to the internal user ID. Staleness is bounded by
// the TTL, which is far shorter than a token's lifetime.
type CachedUserResolver struct {
	next UserResolver
	ttl  time.Duration
	max  int

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	user    db.User
	expires time.Time
}

func NewCachedUserResolver(next UserResolver, ttl time.Duration, maxEntries int) *CachedUserResolver {
	return &CachedUserResolver{
		next:    next,
		ttl:     ttl,
		max:     maxEntries,
		entries: make(map[string]cacheEntry),
	}
}

func (c *CachedUserResolver) GetUserByEmail(ctx context.Context, sub string) (db.User, error) {
	c.mu.Lock()
	if e, ok := c.entries[sub]; ok && time.Now().Before(e.expires) {
		user := e.user
		c.mu.Unlock()
		return user, nil
	}
	c.mu.Unlock()

	user, err := c.next.GetUserByEmail(ctx, sub)
	if err != nil {
		return user, err
	}

	c.mu.Lock()
	if len(c.entries) >= c.max {
		c.evictLocked()
	}
	if len(c.entries) < c.max {
		c.entries[sub] = cacheEntry{user: user, expires: time.Now().Add(c.ttl)}
	}
	c.mu.Unlock()

	return user, nil
}

// evictLocked makes room for a new entry: expired entries are dropped first;
// if the cache is still full, the entry expiring soonest is evicted, which
// approximates LRU without tracking access order.
func (c *CachedUserResolver) evictLocked() {
	now := time.Now()
	oldestKey := ""
	var oldestExpiry time.Time
	found := false

	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
			continue
		}
		if !found || e.expires.Before(oldestExpiry) {
			oldestKey, oldestExpiry, found = k, e.expires, true
		}
	}
	if found && len(c.entries) >= c.max {
		delete(c.entries, oldestKey)
	}
}
