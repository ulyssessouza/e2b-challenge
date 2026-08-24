package middleware

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/MicahParks/keyfunc/v3"
)

const (
	ContextUserID    = "user_id"
	ContextUserEmail = "user_email"
)

func JWTAuth(kf keyfunc.Keyfunc) echo.MiddlewareFunc {
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

			token, err := jwt.Parse(parts[1], kf.Keyfunc)
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

			c.Set(ContextUserID, sub)
			if email, ok := claims["email"].(string); ok {
				c.Set(ContextUserEmail, email)
			}

			return next(c)
		}
	}
}