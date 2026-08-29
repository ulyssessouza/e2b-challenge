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

// ratelimitScript increments the window counter and sets its TTL in a single
// atomic operation. Doing INCR and EXPIRE as separate commands can leave the
// key without a TTL if the process dies in between, which would both leak the
// key and permanently rate-limit the user for that window.
var ratelimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {count, redis.call('PTTL', KEYS[1])}
`)

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

func enforceRateLimit(next echo.HandlerFunc, c echo.Context, rdb *redis.Client, key string, limit int, failOpen bool) error {
	ctx := c.Request().Context()

	res, err := ratelimitScript.Run(ctx, rdb, []string{key}, (time.Minute / time.Millisecond)).Slice()
	if err != nil {
		if failOpen {
			slog.Warn("rate limiter: redis unreachable, allowing request", "error", err)
			return next(c)
		}
		slog.Error("rate limiter: redis unreachable, rejecting request", "error", err)
		return echo.NewHTTPError(http.StatusServiceUnavailable, "rate limiter unavailable")
	}

	count, ttlMs := toInt64(res[0]), toInt64(res[1])

	c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	remaining := int64(limit) - count
	if remaining < 0 {
		remaining = 0
	}
	c.Response().Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

	if count > int64(limit) {
		retryAfter := int64(math.Ceil(float64(ttlMs) / 1000))
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Response().Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
		return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
	}

	return next(c)
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
