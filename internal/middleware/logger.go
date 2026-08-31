package middleware

import (
	"errors"
	"log/slog"
	"net/http"
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
				// The HTTPErrorHandler runs after the middleware chain
				// returns, so for failed requests the response status is
				// still the zero default here — derive it from the error.
				status := c.Response().Status
				if err != nil {
					var he *echo.HTTPError
					if errors.As(err, &he) {
						status = he.Code
					} else {
						status = http.StatusInternalServerError
					}
				}

				// Echo's RequestID middleware sets the id on the response
				// header (and honors an inbound X-Request-ID), not on the
				// context — read it after the handler has run.
				slog.Info("request",
					"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
					"method", c.Request().Method,
					"path", c.Request().URL.Path,
					"status", status,
					"duration_ms", time.Since(start).Milliseconds(),
				)
			}
			return err
		}
	}
}
