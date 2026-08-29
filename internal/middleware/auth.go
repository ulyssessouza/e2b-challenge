package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

const (
	ContextUserID    = "user_id"
	ContextUserEmail = "user_email"
	// ContextProjectRole is set by ProjectMembership to the caller's role
	// within the project ("owner" or "member").
	ContextProjectRole = "project_role"
)

type UserResolver interface {
	GetUserByOAuthSub(ctx context.Context, sub string) (db.User, error)
}

func JWTAuth(kf keyfunc.Keyfunc, users UserResolver, issuer, audience string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
			}

			if kf == nil {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "authentication service unavailable")
			}

			token, err := jwt.Parse(parts[1], kf.Keyfunc,
				jwt.WithValidMethods([]string{"RS256"}),
				jwt.WithExpirationRequired(),
				jwt.WithIssuer(issuer),
				jwt.WithAudience(audience),
			)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
			}

			sub, _ := claims["sub"].(string)
			if sub == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "token missing subject")
			}

			user, err := users.GetUserByOAuthSub(c.Request().Context(), sub)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "database error")
			}

			c.Set(ContextUserID, user.ID)
			c.Set(ContextUserEmail, user.Email)

			return next(c)
		}
	}
}
