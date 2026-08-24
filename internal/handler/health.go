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
}

func NewHealthCheck(db *sql.DB, rdb *redis.Client) *HealthCheck {
	return &HealthCheck{db: db, rdb: rdb}
}

type healthStatus struct {
	Status string                `json:"status"`
	Checks map[string]checkState `json:"checks"`
}

type checkState struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (h *HealthCheck) Check(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]checkState)
	overall := "ok"

	dbStatus, dbMsg := h.checkDB(ctx)
	checks["database"] = checkState{Status: dbStatus, Message: dbMsg}
	if dbStatus != "up" {
		overall = "degraded"
	}

	redisStatus, redisMsg := h.checkRedis(ctx)
	checks["redis"] = checkState{Status: redisStatus, Message: redisMsg}
	if redisStatus != "up" && overall == "ok" {
		overall = "degraded"
	}

	statusCode := http.StatusOK
	if overall == "degraded" && dbStatus != "up" {
		statusCode = http.StatusServiceUnavailable
	}

	return c.JSON(statusCode, healthStatus{
		Status: overall,
		Checks: checks,
	})
}

func (h *HealthCheck) checkDB(ctx context.Context) (string, string) {
	if err := h.db.PingContext(ctx); err != nil {
		slog.Warn("health check: database unreachable", "error", err)
		return "down", err.Error()
	}
	return "up", ""
}

func (h *HealthCheck) checkRedis(ctx context.Context) (string, string) {
	if h.rdb == nil {
		return "degraded", "not configured"
	}
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("health check: redis unreachable", "error", err)
		return "degraded", err.Error()
	}
	return "up", ""
}