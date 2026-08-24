package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

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
				return echo.NewHTTPError(http.StatusInternalServerError, "rate limiter error")
			}

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