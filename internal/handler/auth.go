package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/service"
)

const stateCookieName = "oauth_state"

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Login(c echo.Context) error {
	state, err := generateState()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate state")
	}

	c.SetCookie(&http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	authURL := fmt.Sprintf(
		"%s/oauth2/auth?client_id=%s&response_type=code&scope=openid&state=%s&redirect_uri=%s",
		h.svc.HydraPublicURL(),
		url.QueryEscape(h.svc.OAuthClientID()),
		url.QueryEscape(state),
		url.QueryEscape(h.svc.OAuthRedirectURI()),
	)
	return c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) Callback(c echo.Context) error {
	state := c.QueryParam("state")
	cookie, err := c.Cookie(stateCookieName)
	if err != nil || state == "" || cookie.Value == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid state parameter")
	}

	if oauthErr := c.QueryParam("error"); oauthErr != "" {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("authorization failed: %s", oauthErr))
	}

	code := c.QueryParam("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing code parameter")
	}

	c.SetCookie(&http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

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

	// The token came from Hydra's token endpoint, so its signature was
	// verified there — but still sanity-check the identity-relevant claims
	// to catch a misconfigured provider before creating user records.
	if iss, _ := claims["iss"].(string); iss != h.svc.HydraPublicURL() {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected token issuer")
	}
	if cid, _ := claims["client_id"].(string); cid != h.svc.OAuthClientID() {
		return echo.NewHTTPError(http.StatusInternalServerError, "token issued for a different client")
	}
	if exp, ok := claims["exp"].(float64); !ok || time.Now().Unix() >= int64(exp) {
		return echo.NewHTTPError(http.StatusInternalServerError, "token expired or missing expiration")
	}

	user, err := h.svc.FindOrCreateUser(c.Request().Context(), sub)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("user lookup failed: %v", err))
	}

	c.Response().Header().Set("Cache-Control", "no-store")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
		"user_id":      user.ID,
		"email":        user.Email,
	})
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
