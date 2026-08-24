package middleware

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

var redisDown atomic.Bool

func RedisDown() bool {
	return redisDown.Load()
}

func RateLimiter(rdb *redis.Client, limit int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := c.Get(ContextUserID).(string)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
			}

			now := time.Now().UTC()
			window := now.Format(":2006-01-02T15:04")
			key := "ratelimit:" + userID + window
			ctx := c.Request().Context()

			count, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				redisDown.Store(true)
				slog.Warn("rate limiter: redis unreachable, allowing request", "error", err)
				return next(c)
			}
			redisDown.Store(false)

			if count == 1 {
				rdb.Expire(ctx, key, time.Minute)
			}

			if count > int64(limit) {
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
			}

			return next(c)
		}
	}
}