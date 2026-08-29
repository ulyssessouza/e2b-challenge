package middleware

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

func RateLimiter(rdb *redis.Client, limit int, failOpen bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := c.Get(ContextUserID).(string)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
			}

			now := time.Now().UTC()
			window := now.Format(":2006-01-02T15:04")
			key := "ratelimit:user:" + userID + window

			return enforceRateLimit(next, c, rdb, key, limit, failOpen)
		}
	}
}

// IPRateLimiter throttles unauthenticated routes (e.g. /auth/*) by client IP.
func IPRateLimiter(rdb *redis.Client, limit int, failOpen bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			now := time.Now().UTC()
			window := now.Format(":2006-01-02T15:04")
			key := "ratelimit:ip:" + c.RealIP() + window

			return enforceRateLimit(next, c, rdb, key, limit, failOpen)
		}
	}
}

// enforceRateLimit counts requests in fixed one-minute windows. INCR and
// EXPIRE are deliberately issued as separate, non-atomic commands: if the
// process dies in between, the window key is left without a TTL. The worst
// case is one leaked key (and a throttled user for that window) until Redis
// is flushed — an accepted trade-off here; a Lua script would make it atomic
// if it ever matters.
func enforceRateLimit(next echo.HandlerFunc, c echo.Context, rdb *redis.Client, key string, limit int, failOpen bool) error {
	ctx := c.Request().Context()

	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		if failOpen {
			slog.Warn("rate limiter: redis unreachable, allowing request", "error", err)
			return next(c)
		}
		slog.Error("rate limiter: redis unreachable, rejecting request", "error", err)
		return echo.NewHTTPError(http.StatusServiceUnavailable, "rate limiter unavailable")
	}

	if count == 1 {
		rdb.Expire(ctx, key, time.Minute)
	}

	c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	remaining := max(int64(limit)-count, 0)
	c.Response().Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

	if count > int64(limit) {
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			ttl = time.Minute
		}
		retryAfter := max(int64(math.Ceil(ttl.Seconds())), 1)
		c.Response().Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
		return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
	}

	return next(c)
}
