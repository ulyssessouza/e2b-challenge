package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

// MembershipChecker resolves whether a user is a member of a project. A nil
// role (Valid == false) means the project exists but the user is not a member.
type MembershipChecker interface {
	GetProjectMembership(ctx context.Context, arg db.GetProjectMembershipParams) (sql.NullString, error)
}

func ProjectMembership(q MembershipChecker) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("id")
			userID, ok := c.Get(ContextUserID).(string)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
			}

			role, err := q.GetProjectMembership(c.Request().Context(), db.GetProjectMembershipParams{
				ID:     projectID,
				UserID: userID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return echo.NewHTTPError(http.StatusNotFound, "project not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "database error")
			}
			if !role.Valid {
				return echo.NewHTTPError(http.StatusForbidden, "not a member of this project")
			}

			c.Set(ContextProjectRole, role.String)

			return next(c)
		}
	}
}
