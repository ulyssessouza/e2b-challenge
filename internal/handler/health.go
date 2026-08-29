package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type HealthCheck struct {
	db  *sql.DB
	rdb *redis.Client
	// authReady reports whether the JWKS keyfunc was loaded; without it every
	// authenticated route returns 503, so health must reflect it.
	authReady bool
}

func NewHealthCheck(db *sql.DB, rdb *redis.Client, authReady bool) *HealthCheck {
	return &HealthCheck{db: db, rdb: rdb, authReady: authReady}
}

type healthStatus struct {
	Status string                `json:"status"`
	Checks map[string]checkState `json:"checks"`
}

type checkState struct {
	Status string `json:"status"`
}

func (h *HealthCheck) Check(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]checkState)
	overall := "ok"

	dbStatus := h.checkDB(ctx)
	checks["database"] = checkState{Status: dbStatus}

	redisStatus := h.checkRedis(ctx)
	checks["redis"] = checkState{Status: redisStatus}

	authStatus := "up"
	if !h.authReady {
		authStatus = "down"
	}
	checks["auth"] = checkState{Status: authStatus}

	statusCode := http.StatusOK
	overall = "ok"
	for _, s := range []string{dbStatus, redisStatus, authStatus} {
		if s != "up" {
			overall = "degraded"
		}
	}
	if dbStatus != "up" || authStatus != "up" {
		statusCode = http.StatusServiceUnavailable
	}

	return c.JSON(statusCode, healthStatus{
		Status: overall,
		Checks: checks,
	})
}

func (h *HealthCheck) checkDB(ctx context.Context) string {
	if err := h.db.PingContext(ctx); err != nil {
		slog.Warn("health check: database unreachable", "error", err)
		return "down"
	}
	return "up"
}

func (h *HealthCheck) checkRedis(ctx context.Context) string {
	if h.rdb == nil {
		return "degraded"
	}
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("health check: redis unreachable", "error", err)
		return "degraded"
	}
	return "up"
}
