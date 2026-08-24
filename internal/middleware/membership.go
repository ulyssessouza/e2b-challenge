package middleware

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

func ProjectMembership(q *db.Queries) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("id")
			userID := c.Get(ContextUserID).(string)

			_, err := q.GetProjectByID(c.Request().Context(), projectID)
			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusNotFound, "project not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "database error")
			}

			_, err = q.GetProjectMember(c.Request().Context(), db.GetProjectMemberParams{
				ProjectID: projectID,
				UserID:    userID,
			})
			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this project")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "database error")
			}

			return next(c)
		}
	}
}