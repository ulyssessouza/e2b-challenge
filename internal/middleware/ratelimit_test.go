package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

func requireRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("redis not available — integration test")
	}
	return rdb
}

func runRateLimit(t *testing.T, rdb *redis.Client, userID string, limit int, failOpen bool) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserID, userID)

	handler := RateLimiter(rdb, limit, failOpen)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	err := handler(c)
	return rec, err
}

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rdb := requireRedis(t)
	defer rdb.Close()

	rec, err := runRateLimit(t, rdb, "user-"+strconv.FormatInt(time.Now().UnixNano(), 10), 1000, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimiterRejectsOverLimit(t *testing.T) {
	rdb := requireRedis(t)
	defer rdb.Close()

	userID := "user-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	// Exhaust a tight limit; the third call must be rejected.
	var lastRec *httptest.ResponseRecorder
	var lastErr error
	for i := 0; i < 3; i++ {
		lastRec, lastErr = runRateLimit(t, rdb, userID, 2, true)
	}

	httpErr, ok := lastErr.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %v", lastErr)
	}
	if lastRec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429")
	}
	if lastRec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining 0, got %q", lastRec.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRateLimiterFailClosedWhenRedisDown(t *testing.T) {
	// Nothing listens on this port; fail-closed must reject the request.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:5390", DialTimeout: 200 * time.Millisecond})
	defer rdb.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	reqCtx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserID, "user-fail-closed")

	handler := RateLimiter(rdb, 1000, false)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with fail-closed, got %v", err)
	}
}
