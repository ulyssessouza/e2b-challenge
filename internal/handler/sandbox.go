package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/middleware"
	"e2b-challenge/internal/pagination"
	"e2b-challenge/internal/service"
)

type SandboxHandler struct {
	svc *service.SandboxService
}

func NewSandboxHandler(svc *service.SandboxService) *SandboxHandler {
	return &SandboxHandler{svc: svc}
}

func (h *SandboxHandler) List(c echo.Context) error {
	projectID := c.Param("id")
	p := parsePagination(c)

	sandboxes, total, err := h.svc.ListByProject(c.Request().Context(), projectID, p)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, pagination.NewResponse(sandboxes, p, total))
}

func (h *SandboxHandler) Create(c echo.Context) error {
	projectID := c.Param("id")
	userID := c.Get(middleware.ContextUserID).(string)

	sandbox, err := h.svc.Create(c.Request().Context(), projectID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, sandbox)
}

func (h *SandboxHandler) Stop(c echo.Context) error {
	sandboxID := c.Param("id")
	userID := c.Get(middleware.ContextUserID).(string)

	err := h.svc.Stop(c.Request().Context(), sandboxID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if strings.Contains(err.Error(), "not a member") {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		if strings.Contains(err.Error(), "already stopped") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		if strings.Contains(err.Error(), "conflict") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}