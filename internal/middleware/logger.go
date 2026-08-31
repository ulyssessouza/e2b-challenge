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
				// Echo's RequestID middleware sets the id on the response
				// header (and honors an inbound X-Request-ID), not on the
				// context — read it after the handler has run.
				slog.Info("request",
					"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
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
