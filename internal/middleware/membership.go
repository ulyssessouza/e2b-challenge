package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

// MembershipChecker resolves the caller's relationship to a project in a
// single round-trip: whether the project exists, and whether the caller is
// a member (owners are members too — role is derived from projects.owner_id).
type MembershipChecker interface {
	GetProjectMembership(ctx context.Context, arg db.GetProjectMembershipParams) (db.GetProjectMembershipRow, error)
}

func ProjectMembership(q MembershipChecker) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("id")
			userID, ok := c.Get(ContextUserID).(string)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
			}

			membership, err := q.GetProjectMembership(c.Request().Context(), db.GetProjectMembershipParams{
				CallerID:  userID,
				ProjectID: projectID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return echo.NewHTTPError(http.StatusNotFound, "project not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "database error")
			}
			if !membership.MemberUserID.Valid {
				return echo.NewHTTPError(http.StatusForbidden, "not a member of this project")
			}

			role := "member"
			if membership.IsOwner {
				role = "owner"
			}
			c.Set(ContextProjectRole, role)

			return next(c)
		}
	}
}
