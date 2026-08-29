package server

import (
	"database/sql"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"

	"e2b-challenge/internal/config"
	"e2b-challenge/internal/db"
	"e2b-challenge/internal/handler"
	mid "e2b-challenge/internal/middleware"
	"e2b-challenge/internal/service"
)

func New(cfg *config.Config, sqlDB *sql.DB, rdb *redis.Client, kf keyfunc.Keyfunc) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.HTTPErrorHandler

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.BodyLimit("1M"))
	e.Use(mid.RequestLogger())

	queries := db.New(sqlDB)

	authSvc := service.NewAuthService(queries, cfg)
	projectSvc := service.NewProjectService(queries, sqlDB)
	sandboxSvc := service.NewSandboxService(queries, cfg.MaxRunningSandboxesPerProject)

	authH := handler.NewAuthHandler(authSvc)
	projectH := handler.NewProjectHandler(projectSvc)
	sandboxH := handler.NewSandboxHandler(sandboxSvc)
	healthH := handler.NewHealthCheck(sqlDB, rdb, kf != nil)

	e.GET("/health", healthH.Check)
	e.GET("/auth/login", authH.Login, mid.IPRateLimiter(rdb, cfg.AuthRateLimitPerMin, cfg.RateLimitFailOpen))
	e.GET("/auth/callback", authH.Callback, mid.IPRateLimiter(rdb, cfg.AuthRateLimitPerMin, cfg.RateLimitFailOpen))

	r := e.Group("")
	r.Use(mid.JWTAuth(kf, mid.NewCachedUserResolver(queries, time.Minute, 10000), cfg.HydraPublicURL, cfg.OAuthClientID))
	r.Use(mid.RateLimiter(rdb, cfg.RateLimitPerMin, cfg.RateLimitFailOpen))

	r.GET("/v1/projects", projectH.List)
	r.POST("/v1/projects", projectH.Create)

	projectGroup := r.Group("/v1/projects/:id")
	projectGroup.Use(mid.ProjectMembership(queries))
	projectGroup.GET("", projectH.Get)
	projectGroup.POST("/members", projectH.AddMember)
	projectGroup.GET("/sandboxes", sandboxH.List)
	projectGroup.POST("/sandboxes", sandboxH.Create)

	r.DELETE("/v1/sandboxes/:id", sandboxH.Stop)

	return e
}
