package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/config"
	"e2b-challenge/internal/service"
)

func newTestAuthHandler(t *testing.T) (*AuthHandler, *echo.Echo) {
	t.Helper()
	cfg := &config.Config{
		HydraPublicURL:   "http://hydra:4444",
		OAuthClientID:    "e2b-assignment",
		OAuthRedirectURI: "http://localhost:8080/auth/callback",
	}
	svc := service.NewAuthService(nil, cfg)
	return NewAuthHandler(svc), echo.New()
}

func TestLoginIncludesState(t *testing.T) {
	h, e := newTestAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}

	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("invalid Location header: %v", err)
	}
	state := u.Query().Get("state")
	if len(state) < 8 {
		t.Fatalf("expected state >= 8 chars in authorize URL, got %q", state)
	}

	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "oauth_state" && ck.Value == state {
			return
		}
	}
	t.Fatalf("expected oauth_state cookie matching state %q", state)
}

func TestCallbackRejectsMissingState(t *testing.T) {
	h, e := newTestAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Callback(c)
	var he *echo.HTTPError
	ok := errors.As(err, &he)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %v", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", he.Code)
	}
}

func TestCallbackRejectsMismatchedState(t *testing.T) {
	h, e := newTestAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=aaaaaaaaaaaaaaaa", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "bbbbbbbbbbbbbbbb"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Callback(c)
	var he *echo.HTTPError
	ok := errors.As(err, &he)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %v", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", he.Code)
	}
}
