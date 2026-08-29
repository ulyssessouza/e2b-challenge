package handler

import (
	"errors"
	"net/http"

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

	return c.JSON(http.StatusOK, pagination.NewResponse(toSandboxDTOs(sandboxes), p, total))
}

type createSandboxRequest struct {
	Name      string `json:"name"`
	SandboxID string `json:"sandbox_id"`
}

func (h *SandboxHandler) Create(c echo.Context) error {
	projectID := c.Param("id")
	userID := c.Get(middleware.ContextUserID).(string)

	var req createSandboxRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.Name) > maxNameLength {
		return echo.NewHTTPError(http.StatusBadRequest, "name too long")
	}

	sandbox, created, err := h.svc.Create(c.Request().Context(), projectID, userID, req.Name, req.SandboxID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		case errors.Is(err, service.ErrConflict):
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		case errors.Is(err, service.ErrQuotaExceeded):
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.JSON(status, toSandboxDTO(*sandbox))
}

func (h *SandboxHandler) Stop(c echo.Context) error {
	sandboxID := c.Param("id")
	userID := c.Get(middleware.ContextUserID).(string)

	err := h.svc.Stop(c.Request().Context(), sandboxID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		case errors.Is(err, service.ErrConflict):
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.NoContent(http.StatusNoContent)
}
