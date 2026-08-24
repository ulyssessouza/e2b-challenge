package handler

import (
    "fmt"
    "net/http"

    "github.com/golang-jwt/jwt/v5"
    "github.com/labstack/echo/v4"

    "e2b-challenge/internal/service"
)

type AuthHandler struct {
    svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
    return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Login(c echo.Context) error {
    authURL := fmt.Sprintf(
        "%s/oauth2/auth?client_id=%s&response_type=code&scope=openid&redirect_uri=%s",
        h.svc.HydraPublicURL(),
        h.svc.OAuthClientID(),
        h.svc.OAuthRedirectURI(),
    )
    return c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) Callback(c echo.Context) error {
    code := c.QueryParam("code")
    if code == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "missing code parameter")
    }

    accessToken, err := h.svc.ExchangeCode(c.Request().Context(), code)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("token exchange failed: %v", err))
    }

    token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, "failed to decode token")
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return echo.NewHTTPError(http.StatusInternalServerError, "invalid token claims")
    }

    sub, _ := claims["sub"].(string)
    if sub == "" {
        return echo.NewHTTPError(http.StatusInternalServerError, "token missing subject")
    }

    user, err := h.svc.FindOrCreateUser(c.Request().Context(), sub)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("user lookup failed: %v", err))
    }

    return c.JSON(http.StatusOK, map[string]interface{}{
        "access_token": accessToken,
        "user_id":      user.ID,
        "email":        user.Email,
    })
}