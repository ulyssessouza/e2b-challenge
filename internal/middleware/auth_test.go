package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

type stubUserResolver struct {
	user db.User
	err  error
}

func (s stubUserResolver) GetUserByEmail(_ context.Context, _ string) (db.User, error) {
	return s.user, s.err
}

func testKeyfunc(t *testing.T) (keyfunc.Keyfunc, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwk, err := jwkset.NewJWKFromKey(priv.Public(), jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: "test-key"},
	})
	if err != nil {
		t.Fatalf("creating jwk: %v", err)
	}
	storage := jwkset.NewMemoryStorage()
	if err := storage.KeyWrite(context.Background(), jwk); err != nil {
		t.Fatalf("storing jwk: %v", err)
	}
	k, err := keyfunc.New(keyfunc.Options{Ctx: context.Background(), Storage: storage})
	if err != nil {
		t.Fatalf("creating keyfunc: %v", err)
	}
	return k, priv
}

func signToken(t *testing.T, priv *rsa.PrivateKey, sub string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub,
		"iss": "http://localhost:4444",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-key"
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func doJWTAuth(t *testing.T, kf keyfunc.Keyfunc, users UserResolver, token string) (echo.Context, *echo.HTTPError) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	handler := JWTAuth(kf, users)(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
		}
		return c, he
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	return c, nil
}

func TestJWTAuthMissingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(nil, nil)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", httpErr.Code)
	}
}

func TestJWTAuthResolvesSubjectToInternalUserID(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{
		user: db.User{ID: "4ec6c108-1ab0-48f0-9e23-11955c360310", Email: "foo@bar.com"},
	}
	token := signToken(t, priv, "foo@bar.com")

	c, he := doJWTAuth(t, kf, stub, token)
	if he != nil {
		t.Fatalf("unexpected error: %v", he)
	}
	if got := c.Get(ContextUserID); got != "4ec6c108-1ab0-48f0-9e23-11955c360310" {
		t.Fatalf("expected ContextUserID to be internal user UUID, got %v", got)
	}
	if got := c.Get(ContextUserEmail); got != "foo@bar.com" {
		t.Fatalf("expected ContextUserEmail foo@bar.com, got %v", got)
	}
}

func TestJWTAuthRejectsTokenForUnknownUser(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{err: sql.ErrNoRows}
	token := signToken(t, priv, "nobody@example.com")

	_, he := doJWTAuth(t, kf, stub, token)
	if he == nil {
		t.Fatal("expected error, got nil")
	}
	if he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", he.Code)
	}
}
