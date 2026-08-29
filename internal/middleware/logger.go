package middleware

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

// RequestLogger emits one structured log line per request. Only errors were
// logged before, so requests were invisible in operation.
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)

			if c.Path() != "/health" {
				slog.Info("request",
					"request_id", c.Get(echo.HeaderXRequestID),
					"method", c.Request().Method,
					"path", c.Request().URL.Path,
					"status", c.Response().Status,
					"duration_ms", time.Since(start).Milliseconds(),
				)
			}
			return err
		}
	}
}
