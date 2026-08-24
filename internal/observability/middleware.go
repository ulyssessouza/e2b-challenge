package observability

import (
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	instrumentsOnce sync.Once
	requestCount    metric.Int64Counter
	requestDur      metric.Int64Histogram
	requestErrors   metric.Int64Counter
)

func initInstruments() {
	instrumentsOnce.Do(func() {
		meter := otel.Meter("e2b-sandbox-api")
		var err error

		requestCount, err = meter.Int64Counter("http.requests.total",
			metric.WithDescription("Total number of HTTP requests"),
			metric.WithUnit("{request}"),
		)
		if err != nil {
			panic(err)
		}

		requestDur, err = meter.Int64Histogram("http.request.duration_ms",
			metric.WithDescription("HTTP request duration in milliseconds"),
			metric.WithUnit("ms"),
		)
		if err != nil {
			panic(err)
		}

		requestErrors, err = meter.Int64Counter("http.requests.errors",
			metric.WithDescription("Number of failed HTTP requests"),
			metric.WithUnit("{error}"),
		)
		if err != nil {
			panic(err)
		}
	})
}

func TracingMiddleware(serviceName string) echo.MiddlewareFunc {
	return otelecho.Middleware(serviceName)
}

func MetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			initInstruments()

			start := time.Now()
			err := next(c)
			dur := time.Since(start)

			attrs := []attribute.KeyValue{
				attribute.String("http.method", c.Request().Method),
				attribute.String("http.route", c.Path()),
				attribute.Int("http.status_code", c.Response().Status),
			}

			ctx := c.Request().Context()
			requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
			requestDur.Record(ctx, dur.Milliseconds(), metric.WithAttributes(attrs...))

			if err != nil || c.Response().Status >= 500 {
				requestErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
			}

			return err
		}
	}
}