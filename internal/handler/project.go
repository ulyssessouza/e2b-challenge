package handler

import (
    "net/http"

    "github.com/labstack/echo/v4"

    "e2b-challenge/internal/middleware"
    "e2b-challenge/internal/service"
)

type ProjectHandler struct {
    svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
    return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) List(c echo.Context) error {
    userID := c.Get(middleware.ContextUserID).(string)

    projects, err := h.svc.ListByUser(c.Request().Context(), userID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusOK, projects)
}

func (h *ProjectHandler) Create(c echo.Context) error {
    userID := c.Get(middleware.ContextUserID).(string)

    var req struct {
        Name string `json:"name"`
    }
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
    }
    if req.Name == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "name is required")
    }

    project, err := h.svc.Create(c.Request().Context(), req.Name, userID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusCreated, project)
}

func (h *ProjectHandler) Get(c echo.Context) error {
    id := c.Param("id")

    project, err := h.svc.GetByID(c.Request().Context(), id)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    if project == nil {
        return echo.NewHTTPError(http.StatusNotFound, "project not found")
    }

    return c.JSON(http.StatusOK, project)
}

func (h *ProjectHandler) AddMember(c echo.Context) error {
    projectID := c.Param("id")

    var req struct {
        Email string `json:"email"`
    }
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
    }
    if req.Email == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "email is required")
    }

    user, err := h.svc.AddMember(c.Request().Context(), projectID, req.Email, "member")
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusOK, map[string]string{
        "user_id": user.ID,
        "email":   user.Email,
    })
}