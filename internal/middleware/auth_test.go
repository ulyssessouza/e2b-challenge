package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
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

func (s stubUserResolver) GetUserByOAuthSub(_ context.Context, _ string) (db.User, error) {
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
	return signTokenWithClaims(t, priv, func(claims jwt.MapClaims) {
		claims["sub"] = sub
	})
}

func signTokenWithClaims(t *testing.T, priv *rsa.PrivateKey, mutate func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":       "foo@bar.com",
		"iss":       "http://localhost:4444",
		"aud":       []string{},
		"client_id": "e2b-assignment",
		"iat":       time.Now().Add(-time.Minute).Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	if mutate != nil {
		mutate(claims)
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
	return doJWTAuthWithIssuer(t, kf, users, "http://localhost:4444", "e2b-assignment", token)
}

func doJWTAuthWithIssuer(t *testing.T, kf keyfunc.Keyfunc, users UserResolver, issuer, audience, token string) (echo.Context, *echo.HTTPError) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	handler := JWTAuth(kf, users, issuer, audience)(func(c echo.Context) error {
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

	handler := JWTAuth(nil, nil, "http://localhost:4444", "e2b-assignment")(func(c echo.Context) error {
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

func TestJWTAuthRejectsWrongIssuer(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}

	token := signTokenWithClaims(t, priv, func(claims jwt.MapClaims) {
		claims["iss"] = "http://evil.example"
	})

	_, he := doJWTAuth(t, kf, stub, token)
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for foreign issuer, got %+v", he)
	}
}

func TestJWTAuthRejectsWrongClientID(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}

	token := signTokenWithClaims(t, priv, func(claims jwt.MapClaims) {
		claims["client_id"] = "another-app"
	})

	_, he := doJWTAuth(t, kf, stub, token)
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for foreign client_id, got %+v", he)
	}
}

func TestJWTAuthRejectsMissingClientID(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}

	token := signTokenWithClaims(t, priv, func(claims jwt.MapClaims) {
		delete(claims, "client_id")
	})

	_, he := doJWTAuth(t, kf, stub, token)
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token without client_id, got %+v", he)
	}
}

func TestJWTAuthRejectsTokenWithoutExp(t *testing.T) {
	kf, priv := testKeyfunc(t)
	stub := stubUserResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}

	token := signTokenWithClaims(t, priv, func(claims jwt.MapClaims) {
		delete(claims, "exp")
	})

	_, he := doJWTAuth(t, kf, stub, token)
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token without exp, got %+v", he)
	}
}

func TestJWTAuthRejectsUnexpectedAlgorithm(t *testing.T) {
	kf, _ := testKeyfunc(t)
	stub := stubUserResolver{user: db.User{ID: "u1", Email: "foo@bar.com"}}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	claims := jwt.MapClaims{
		"sub": "foo@bar.com",
		"iss": "http://localhost:4444",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = "test-key"
	signed, err := tok.SignedString(ecKey)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}

	_, he := doJWTAuth(t, kf, stub, signed)
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for ES256 token, got %+v", he)
	}
}
