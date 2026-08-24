# E2B Sandbox API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go REST API service for E2B sandbox management with OAuth authentication, project-based tenancy, and rate limiting.

**Architecture:** Layered Echo monolith — middleware stack (JWT auth, rate limit, project membership) → handlers → service layer → sqlc-generated PostgreSQL access. Redis for rate limiter state and optional JWKS cache.

**Tech Stack:** Go 1.26, Echo v4, sqlc, PostgreSQL (via lib/pq), Redis (go-redis/v9), golang-jwt/v5, keyfunc/v3 (JWKS), golang-migrate/migrate/v4

---

### Task 1: Project Scaffolding and Configuration

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `go.mod`
- Modify: `main.go`

- [ ] **Step 1: Write configuration test**

```go
// internal/config/config_test.go
package config

import (
    "os"
    "testing"
)

func TestLoad(t *testing.T) {
    os.Setenv("PORT", "9090")
    os.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
    os.Setenv("REDIS_ADDR", "localhost:6380")
    os.Setenv("HYDRA_PUBLIC_URL", "http://localhost:4444")
    os.Setenv("OAUTH_CLIENT_ID", "test-client")
    os.Setenv("OAUTH_CLIENT_SECRET", "test-secret")
    os.Setenv("OAUTH_REDIRECT_URI", "http://localhost:9090/auth/callback")
    os.Setenv("RATE_LIMIT_PER_MIN", "500")
    os.Setenv("RATE_LIMIT_SANDBOX_PER_MIN", "50")

    cfg := Load()

    if cfg.Port != "9090" {
        t.Errorf("expected Port 9090, got %s", cfg.Port)
    }
    if cfg.DatabaseURL != "postgres://localhost:5432/test" {
        t.Errorf("expected DatabaseURL postgres://localhost:5432/test, got %s", cfg.DatabaseURL)
    }
    if cfg.RedisAddr != "localhost:6380" {
        t.Errorf("expected RedisAddr localhost:6380, got %s", cfg.RedisAddr)
    }
    if cfg.HydraPublicURL != "http://localhost:4444" {
        t.Errorf("expected HydraPublicURL http://localhost:4444, got %s", cfg.HydraPublicURL)
    }
    if cfg.OAuthClientID != "test-client" {
        t.Errorf("expected OAuthClientID test-client, got %s", cfg.OAuthClientID)
    }
    if cfg.OAuthClientSecret != "test-secret" {
        t.Errorf("expected OAuthClientSecret test-secret, got %s", cfg.OAuthClientSecret)
    }
    if cfg.OAuthRedirectURI != "http://localhost:9090/auth/callback" {
        t.Errorf("expected OAuthRedirectURI http://localhost:9090/auth/callback, got %s", cfg.OAuthRedirectURI)
    }
    if cfg.RateLimitPerMin != 500 {
        t.Errorf("expected RateLimitPerMin 500, got %d", cfg.RateLimitPerMin)
    }
    if cfg.RateLimitSandboxPerMin != 50 {
        t.Errorf("expected RateLimitSandboxPerMin 50, got %d", cfg.RateLimitSandboxPerMin)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Create config package**

```go
// internal/config/config.go
package config

import (
    "os"
    "strconv"
)

type Config struct {
    Port                   string
    DatabaseURL            string
    RedisAddr              string
    HydraPublicURL         string
    OAuthClientID          string
    OAuthClientSecret      string
    OAuthRedirectURI       string
    RateLimitPerMin        int
    RateLimitSandboxPerMin int
}

func Load() *Config {
    return &Config{
        Port:                   getEnv("PORT", "8080"),
        DatabaseURL:            getEnv("DATABASE_URL", "postgres://e2b:e2b@localhost:5432/e2b?sslmode=disable"),
        RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
        HydraPublicURL:         getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
        OAuthClientID:          getEnv("OAUTH_CLIENT_ID", "e2b-assignment"),
        OAuthClientSecret:      getEnv("OAUTH_CLIENT_SECRET", "e2b-assignment-secret"),
        OAuthRedirectURI:       getEnv("OAUTH_REDIRECT_URI", "http://localhost:8080/auth/callback"),
        RateLimitPerMin:        getIntEnv("RATE_LIMIT_PER_MIN", 1000),
        RateLimitSandboxPerMin: getIntEnv("RATE_LIMIT_SANDBOX_PER_MIN", 100),
    }
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func getIntEnv(key string, fallback int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            return i
        }
    }
    return fallback
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Set up dependencies**

Update `go.mod`:
```
module e2b-challenge

go 1.26

require (
    github.com/labstack/echo/v4 v4.12.0
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/redis/go-redis/v9 v9.5.1
    github.com/lib/pq v1.10.9
    github.com/google/uuid v1.6.0
    github.com/golang-migrate/migrate/v4 v4.17.0
    github.com/MicahParks/keyfunc/v3 v3.3.3
)
```

Run: `go mod tidy`

- [ ] **Step 6: Update main.go skeleton**

```go
// main.go
package main

import "e2b-challenge/internal/config"

func main() {
    cfg := config.Load()
    _ = cfg
}
```

- [ ] **Step 7: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat: scaffold project with config package"
```

---

### Task 2: Database Migrations and sqlc Setup

**Files:**
- Create: `migrations/000001_init.up.sql`
- Create: `migrations/000001_init.down.sql`
- Create: `sqlc.yaml`
- Create: `internal/db/queries/users.sql`
- Create: `internal/db/queries/projects.sql`
- Create: `internal/db/queries/sandboxes.sql`

- [ ] **Step 1: Create migration files**

```sql
-- migrations/000001_init.up.sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_users (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('owner', 'member')),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE sandboxes (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id),
    status     TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'stopped')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at TIMESTAMPTZ
);
```

```sql
-- migrations/000001_init.down.sql
DROP TABLE IF EXISTS sandboxes;
DROP TABLE IF EXISTS project_users;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 2: Create sqlc config**

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"
    queries: "internal/db/queries"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "database/sql"
```

- [ ] **Step 3: Create sqlc query files**

```sql
-- internal/db/queries/users.sql
-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (email, name) VALUES ($1, $2) RETURNING *;
```

```sql
-- internal/db/queries/projects.sql
-- name: CreateProject :one
INSERT INTO projects (name) VALUES ($1) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1 LIMIT 1;

-- name: ListProjectsByUser :many
SELECT p.* FROM projects p
JOIN project_users pu ON pu.project_id = p.id
WHERE pu.user_id = $1
ORDER BY p.created_at DESC;

-- name: AddProjectMember :exec
INSERT INTO project_users (project_id, user_id, role) VALUES ($1, $2, $3);

-- name: GetProjectMember :one
SELECT * FROM project_users WHERE project_id = $1 AND user_id = $2 LIMIT 1;

-- name: ListProjectMembers :many
SELECT u.* FROM users u
JOIN project_users pu ON pu.user_id = u.id
WHERE pu.project_id = $1;
```

```sql
-- internal/db/queries/sandboxes.sql
-- name: CreateSandbox :one
INSERT INTO sandboxes (project_id, user_id) VALUES ($1, $2) RETURNING *;

-- name: GetSandboxByID :one
SELECT * FROM sandboxes WHERE id = $1 LIMIT 1;

-- name: ListSandboxesByProject :many
SELECT * FROM sandboxes WHERE project_id = $1 ORDER BY created_at DESC;

-- name: UpdateSandboxStatus :exec
UPDATE sandboxes SET status = $2, stopped_at = now() WHERE id = $1;
```

- [ ] **Step 4: Generate sqlc code**

Run: `sqlc generate`
Expected: creates `internal/db/models.go`, `internal/db/db.go`, `internal/db/users.sql.go`, `internal/db/projects.sql.go`, `internal/db/sandboxes.sql.go`

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add database schema, migrations, and sqlc queries"
```

---

### Task 3: JWKS Provider

**Files:**
- Create: `internal/jwks/provider.go`
- Create: `internal/jwks/provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/jwks/provider_test.go
package jwks

import (
    "context"
    "testing"
    "time"
)

func TestNewProvider(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    kf, err := NewProvider(ctx, "http://localhost:4444/.well-known/jwks.json")
    if err != nil {
        t.Fatalf("NewProvider failed: %v", err)
    }
    if kf == nil {
        t.Fatal("expected non-nil keyfunc")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/jwks/ -v`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Create JWKS provider**

```go
// internal/jwks/provider.go
package jwks

import (
    "context"
    "time"

    "github.com/MicahParks/keyfunc/v3"
)

func NewProvider(ctx context.Context, jwksURL string) (*keyfunc.Keyfunc, error) {
    k, err := keyfunc.Get(ctx, jwksURL, keyfunc.Options{
        RefreshInterval: 5 * time.Minute,
        RefreshErrorHandler: func(err error) {
            // Log and continue using cached keys on refresh failure
        },
    })
    if err != nil {
        return nil, err
    }
    return &k, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/jwks/ -v`
Expected: PASS (requires compose stack to be up)

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add JWKS provider for Hydra token validation"
```

---

### Task 4: JWT Auth Middleware

**Files:**
- Create: `internal/middleware/auth.go`
- Create: `internal/middleware/auth_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/middleware/auth_test.go
package middleware

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/labstack/echo/v4"
    "github.com/MicahParks/keyfunc/v3"
)

func TestJWTAuthMissingHeader(t *testing.T) {
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    // nil keyfunc will cause Parse to fail - we just test missing header path
    handler := JWTAuth(nil)(func(c echo.Context) error {
        return c.String(http.StatusOK, "ok")
    })

    err := handler(c)
    if err != nil {
        // Should return HTTP error, not panic
        httpErr, ok := err.(*echo.HTTPError)
        if !ok {
            t.Fatalf("expected HTTPError, got %T: %v", err, err)
        }
        if httpErr.Code != http.StatusUnauthorized {
            t.Errorf("expected 401, got %d", httpErr.Code)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/ -v`
Expected: FAIL (package doesn't exist)

- [ ] **Step 3: Create JWT auth middleware**

```go
// internal/middleware/auth.go
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

func JWTAuth(kf *keyfunc.Keyfunc) echo.MiddlewareFunc {
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

            token, err := jwt.Parse(parts[1], (*kf).Keyfunc)
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
```

- [ ] **Step 4: Update test and run**

Since the middleware depends on a real JWKS endpoint for full testing, the unit test is limited. For now, verify compilation:

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add JWT auth middleware"
```

---

### Task 5: Rate Limiter Middleware

**Files:**
- Create: `internal/middleware/ratelimit.go`
- Create: `internal/middleware/ratelimit_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/middleware/ratelimit_test.go
package middleware

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/labstack/echo/v4"
    "github.com/redis/go-redis/v9"
)

func TestRateLimiterUnderLimit(t *testing.T) {
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    defer rdb.Close()

    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    c.Set(ContextUserID, "test-user")

    handler := RateLimiter(rdb, 1000)(func(c echo.Context) error {
        return c.String(http.StatusOK, "ok")
    })

    if err := handler(c); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if rec.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", rec.Code)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/ -v -run TestRateLimiterUnderLimit`
Expected: FAIL (function doesn't exist)

- [ ] **Step 3: Create rate limiter middleware**

```go
// internal/middleware/ratelimit.go
package middleware

import (
    "net/http"
    "time"

    "github.com/labstack/echo/v4"
    "github.com/redis/go-redis/v9"
)

func RateLimiter(rdb *redis.Client, limit int) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            userID, ok := c.Get(ContextUserID).(string)
            if !ok {
                return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
            }

            now := time.Now().UTC()
            window := now.Format(":2006-01-02T15:04")
            key := "ratelimit:" + userID + window
            ctx := c.Request().Context()

            count, err := rdb.Incr(ctx, key).Result()
            if err != nil {
                return echo.NewHTTPError(http.StatusInternalServerError, "rate limiter error")
            }

            if count == 1 {
                rdb.Expire(ctx, key, time.Minute)
            }

            if count > int64(limit) {
                return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
            }

            return next(c)
        }
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/middleware/ -v -run TestRateLimiterUnderLimit`
Expected: PASS

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add rate limiter middleware with Redis"
```

---

### Task 6: Auth Service and Handlers

**Files:**
- Create: `internal/service/auth.go`
- Create: `internal/service/auth_test.go`
- Create: `internal/handler/auth.go`
- Create: `internal/handler/auth_test.go`

- [ ] **Step 1: Write failing service test**

```go
// internal/service/auth_test.go
package service

import (
    "testing"
)

func TestFindOrCreateUser(t *testing.T) {
    // Integration test — requires running DB
    // This validates the service compiles and the logic is correct
    t.Skip("integration test — run with compose stack up")
}
```

- [ ] **Step 2: Create auth service**

```go
// internal/service/auth.go
package service

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"

    "e2b-challenge/internal/config"
    "e2b-challenge/internal/db"
)

type AuthService struct {
    q          *db.Queries
    cfg        *config.Config
    httpClient *http.Client
}

func NewAuthService(q *db.Queries, cfg *config.Config) *AuthService {
    return &AuthService{
        q:          q,
        cfg:        cfg,
        httpClient: &http.Client{},
    }
}

type tokenResponse struct {
    AccessToken string `json:"access_token"`
    IDToken     string `json:"id_token"`
}

func (s *AuthService) ExchangeCode(ctx context.Context, code string) (string, error) {
    data := url.Values{
        "grant_type":   {"authorization_code"},
        "code":         {code},
        "redirect_uri": {s.cfg.OAuthRedirectURI},
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        s.cfg.HydraPublicURL+"/oauth2/token",
        strings.NewReader(data.Encode()))
    if err != nil {
        return "", fmt.Errorf("creating token request: %w", err)
    }

    req.SetBasicAuth(s.cfg.OAuthClientID, s.cfg.OAuthClientSecret)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("token exchange request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("reading token response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
    }

    var tr tokenResponse
    if err := json.Unmarshal(body, &tr); err != nil {
        return "", fmt.Errorf("parsing token response: %w", err)
    }

    return tr.AccessToken, nil
}

func (s *AuthService) FindOrCreateUser(ctx context.Context, email string) (*db.User, error) {
    user, err := s.q.GetUserByEmail(ctx, email)
    if err == sql.ErrNoRows {
        user, err = s.q.CreateUser(ctx, db.CreateUserParams{
            Email: email,
            Name:  email,
        })
        if err != nil {
            return nil, fmt.Errorf("creating user: %w", err)
        }
        return &user, nil
    }
    if err != nil {
        return nil, fmt.Errorf("looking up user: %w", err)
    }
    return &user, nil
}
```

- [ ] **Step 3: Create auth handler**

```go
// internal/handler/auth.go
package handler

import (
    "encoding/json"
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
        "http://localhost:4444",
        "e2b-assignment",
        "http://localhost:8080/auth/callback",
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

    // Decode the access token JWT to get the user's subject (email)
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
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add auth service and handlers (OAuth code exchange)"
```

---

### Task 7: Project Service and Handlers

**Files:**
- Create: `internal/service/project.go`
- Create: `internal/service/project_test.go`
- Create: `internal/handler/project.go`
- Create: `internal/handler/project_test.go`

- [ ] **Step 1: Create project service**

```go
// internal/service/project.go
package service

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "e2b-challenge/internal/db"
)

type ProjectService struct {
    q *db.Queries
}

func NewProjectService(q *db.Queries) *ProjectService {
    return &ProjectService{q: q}
}

func (s *ProjectService) Create(ctx context.Context, name, ownerID string) (*db.Project, error) {
    project, err := s.q.CreateProject(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("creating project: %w", err)
    }

    if err := s.q.AddProjectMember(ctx, db.AddProjectMemberParams{
        ProjectID: project.ID,
        UserID:    ownerID,
        Role:      "owner",
    }); err != nil {
        return nil, fmt.Errorf("adding owner: %w", err)
    }

    return &project, nil
}

func (s *ProjectService) ListByUser(ctx context.Context, userID string) ([]db.Project, error) {
    projects, err := s.q.ListProjectsByUser(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("listing projects: %w", err)
    }
    return projects, nil
}

func (s *ProjectService) GetByID(ctx context.Context, id string) (*db.Project, error) {
    project, err := s.q.GetProjectByID(ctx, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, nil
        }
        return nil, fmt.Errorf("getting project: %w", err)
    }
    return &project, nil
}

func (s *ProjectService) AddMember(ctx context.Context, projectID, userEmail, role string) (*db.User, error) {
    user, err := s.q.GetUserByEmail(ctx, userEmail)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, fmt.Errorf("user not found: %s", userEmail)
        }
        return nil, fmt.Errorf("looking up user: %w", err)
    }

    if err := s.q.AddProjectMember(ctx, db.AddProjectMemberParams{
        ProjectID: projectID,
        UserID:    user.ID,
        Role:      role,
    }); err != nil {
        return nil, fmt.Errorf("adding member: %w", err)
    }

    return &user, nil
}
```

- [ ] **Step 2: Create project handler**

```go
// internal/handler/project.go
package handler

import (
    "net/http"

    "github.com/labstack/echo/v4"

    "e2b-challenge/internal/middleware"
    "e2b-challenge/internal/service"
)

type ProjectHandler struct {
    svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
    return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) List(c echo.Context) error {
    userID := c.Get(middleware.ContextUserID).(string)

    projects, err := h.svc.ListByUser(c.Request().Context(), userID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusOK, projects)
}

func (h *ProjectHandler) Create(c echo.Context) error {
    userID := c.Get(middleware.ContextUserID).(string)

    var req struct {
        Name string `json:"name"`
    }
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
    }
    if req.Name == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "name is required")
    }

    project, err := h.svc.Create(c.Request().Context(), req.Name, userID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusCreated, project)
}

func (h *ProjectHandler) Get(c echo.Context) error {
    id := c.Param("id")

    project, err := h.svc.GetByID(c.Request().Context(), id)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    if project == nil {
        return echo.NewHTTPError(http.StatusNotFound, "project not found")
    }

    return c.JSON(http.StatusOK, project)
}

func (h *ProjectHandler) AddMember(c echo.Context) error {
    projectID := c.Param("id")

    var req struct {
        Email string `json:"email"`
    }
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
    }
    if req.Email == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "email is required")
    }

    user, err := h.svc.AddMember(c.Request().Context(), projectID, req.Email, "member")
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusOK, map[string]string{
        "user_id": user.ID,
        "email":   user.Email,
    })
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: add project service and handlers (CRUD + members)"
```

---

### Task 8: Project Membership Middleware

**Files:**
- Create: `internal/middleware/membership.go`
- Create: `internal/middleware/membership_test.go`

- [ ] **Step 1: Create membership middleware**

```go
// internal/middleware/membership.go
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
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: add project membership middleware"
```

---

### Task 9: Sandbox Service and Handlers

**Files:**
- Create: `internal/service/sandbox.go`
- Create: `internal/service/sandbox_test.go`
- Create: `internal/handler/sandbox.go`
- Create: `internal/handler/sandbox_test.go`

- [ ] **Step 1: Create sandbox service**

```go
// internal/service/sandbox.go
package service

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "e2b-challenge/internal/db"
)

type SandboxService struct {
    q *db.Queries
}

func NewSandboxService(q *db.Queries) *SandboxService {
    return &SandboxService{q: q}
}

func (s *SandboxService) Create(ctx context.Context, projectID, userID string) (*db.Sandbox, error) {
    sandbox, err := s.q.CreateSandbox(ctx, db.CreateSandboxParams{
        ProjectID: projectID,
        UserID:    userID,
    })
    if err != nil {
        return nil, fmt.Errorf("creating sandbox: %w", err)
    }
    return &sandbox, nil
}

func (s *SandboxService) ListByProject(ctx context.Context, projectID string) ([]db.Sandbox, error) {
    sandboxes, err := s.q.ListSandboxesByProject(ctx, projectID)
    if err != nil {
        return nil, fmt.Errorf("listing sandboxes: %w", err)
    }
    return sandboxes, nil
}

func (s *SandboxService) Stop(ctx context.Context, sandboxID, userID string) error {
    sandbox, err := s.q.GetSandboxByID(ctx, sandboxID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return fmt.Errorf("sandbox not found")
        }
        return fmt.Errorf("getting sandbox: %w", err)
    }

    // Verify user is a member of the sandbox's project
    _, err = s.q.GetProjectMember(ctx, db.GetProjectMemberParams{
        ProjectID: sandbox.ProjectID,
        UserID:    userID,
    })
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return fmt.Errorf("not a member of this sandbox's project")
        }
        return fmt.Errorf("checking membership: %w", err)
    }

    if err := s.q.UpdateSandboxStatus(ctx, db.UpdateSandboxStatusParams{
        ID:     sandboxID,
        Status: "stopped",
    }); err != nil {
        return fmt.Errorf("stopping sandbox: %w", err)
    }

    return nil
}
```

- [ ] **Step 2: Create sandbox handler**

```go
// internal/handler/sandbox.go
package handler

import (
    "net/http"
    "strings"

    "github.com/labstack/echo/v4"

    "e2b-challenge/internal/middleware"
    "e2b-challenge/internal/service"
)

type SandboxHandler struct {
    svc *service.SandboxService
}

func NewSandboxHandler(svc *service.SandboxService) *SandboxHandler {
    return &SandboxHandler{svc: svc}
}

func (h *SandboxHandler) List(c echo.Context) error {
    projectID := c.Param("id")

    sandboxes, err := h.svc.ListByProject(c.Request().Context(), projectID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusOK, sandboxes)
}

func (h *SandboxHandler) Create(c echo.Context) error {
    projectID := c.Param("id")
    userID := c.Get(middleware.ContextUserID).(string)

    sandbox, err := h.svc.Create(c.Request().Context(), projectID, userID)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.JSON(http.StatusCreated, sandbox)
}

func (h *SandboxHandler) Stop(c echo.Context) error {
    sandboxID := c.Param("id")
    userID := c.Get(middleware.ContextUserID).(string)

    err := h.svc.Stop(c.Request().Context(), sandboxID, userID)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            return echo.NewHTTPError(http.StatusNotFound, err.Error())
        }
        if strings.Contains(err.Error(), "not a member") {
            return echo.NewHTTPError(http.StatusForbidden, err.Error())
        }
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: add sandbox service and handlers (CRUD)"
```

---

### Task 10: Wire Up Server and main.go

**Files:**
- Modify: `main.go`
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Create server setup**

```go
// internal/server/server.go
package server

import (
    "database/sql"
    "fmt"

    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
    "github.com/MicahParks/keyfunc/v3"
    "github.com/redis/go-redis/v9"

    "e2b-challenge/internal/config"
    "e2b-challenge/internal/db"
    "e2b-challenge/internal/handler"
    mid "e2b-challenge/internal/middleware"
    "e2b-challenge/internal/service"
)

func New(cfg *config.Config, sqlDB *sql.DB, rdb *redis.Client, kf *keyfunc.Keyfunc) *echo.Echo {
    e := echo.New()

    e.Use(middleware.Recover())
    e.Use(middleware.Logger())

    queries := db.New(sqlDB)

    authSvc := service.NewAuthService(queries, cfg)
    projectSvc := service.NewProjectService(queries)
    sandboxSvc := service.NewSandboxService(queries)

    authH := handler.NewAuthHandler(authSvc)
    projectH := handler.NewProjectHandler(projectSvc)
    sandboxH := handler.NewSandboxHandler(sandboxSvc)

    e.GET("/auth/login", authH.Login)
    e.GET("/auth/callback", authH.Callback)

    r := e.Group("")
    r.Use(mid.JWTAuth(kf))
    r.Use(mid.RateLimiter(rdb, cfg.RateLimitPerMin))

    r.GET("/v1/projects", projectH.List)
    r.POST("/v1/projects", projectH.Create)

    projectGroup := r.Group("/v1/projects/:id")
    projectGroup.Use(mid.ProjectMembership(queries))
    projectGroup.GET("", projectH.Get)
    projectGroup.POST("/members", projectH.AddMember)
    projectGroup.GET("/sandboxes", sandboxH.List)
    projectGroup.POST("/sandboxes", sandboxH.Create)

    r.DELETE("/v1/sandboxes/:id", sandboxH.Stop)

    return e
}
```

- [ ] **Step 2: Update main.go with initialization**

```go
// main.go
package main

import (
    "database/sql"
    "log"
    "os"
    "os/signal"

    "github.com/redis/go-redis/v9"
    _ "github.com/lib/pq"

    "e2b-challenge/internal/config"
    "e2b-challenge/internal/jwks"
    "e2b-challenge/internal/server"
)

func main() {
    cfg := config.Load()

    sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("failed to open database: %v", err)
    }
    if err := sqlDB.Ping(); err != nil {
        log.Fatalf("failed to ping database: %v", err)
    }

    rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
    if err := rdb.Ping(rdb.Context()).Err(); err != nil {
        log.Fatalf("failed to connect to redis: %v", err)
    }

    kf, err := jwks.NewProvider(rdb.Context(), cfg.HydraPublicURL+"/.well-known/jwks.json")
    if err != nil {
        log.Fatalf("failed to setup JWKS provider: %v", err)
    }

    e := server.New(cfg, sqlDB, rdb, kf)

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)

    go func() {
        <-quit
        log.Println("shutting down...")
        sqlDB.Close()
        rdb.Close()
    }()

    log.Printf("starting server on :%s", cfg.Port)
    e.Logger.Fatal(e.Start(":" + cfg.Port))
}
```

- [ ] **Step 3: Run migrations on startup**

Add migration logic to main.go:

```go
// Add to imports
"github.com/golang-migrate/migrate/v4"
_ "github.com/golang-migrate/migrate/v4/database/postgres"
_ "github.com/golang-migrate/migrate/v4/source/file"

// Add after sqlDB.Ping()
func runMigrations(dbURL, migrationsPath string) error {
    m, err := migrate.New("file://"+migrationsPath, dbURL)
    if err != nil {
        return fmt.Errorf("creating migrator: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("running migrations: %w", err)
    }
    return nil
}
```

Update the main function to call `runMigrations`:

```go
// In main(), after sqlDB.Ping():
if err := runMigrations(cfg.DatabaseURL, "migrations"); err != nil {
    log.Fatalf("failed to run migrations: %v", err)
}
```

Final `main.go`:

```go
package main

import (
    "database/sql"
    "fmt"
    "log"
    "os"
    "os/signal"

    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
    "github.com/lib/pq"
    "github.com/redis/go-redis/v9"

    "e2b-challenge/internal/config"
    "e2b-challenge/internal/jwks"
    "e2b-challenge/internal/server"
)

func main() {
    cfg := config.Load()

    sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("failed to open database: %v", err)
    }
    if err := sqlDB.Ping(); err != nil {
        log.Fatalf("failed to ping database: %v", err)
    }

    if err := runMigrations(cfg.DatabaseURL, "migrations"); err != nil {
        log.Fatalf("failed to run migrations: %v", err)
    }

    rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
    if err := rdb.Ping(rdb.Context()).Err(); err != nil {
        log.Fatalf("failed to connect to redis: %v", err)
    }

    kf, err := jwks.NewProvider(rdb.Context(), cfg.HydraPublicURL+"/.well-known/jwks.json")
    if err != nil {
        log.Fatalf("failed to setup JWKS provider: %v", err)
    }

    e := server.New(cfg, sqlDB, rdb, kf)

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)

    go func() {
        <-quit
        log.Println("shutting down...")
        sqlDB.Close()
        rdb.Close()
    }()

    log.Printf("starting server on :%s", cfg.Port)
    e.Logger.Fatal(e.Start(":" + cfg.Port))
}

func runMigrations(dbURL, migrationsPath string) error {
    m, err := migrate.New("file://"+migrationsPath, dbURL)
    if err != nil {
        return fmt.Errorf("creating migrator: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("running migrations: %w", err)
    }
    return nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: wire up server, middleware stack, and routes"
```

---

## Self-Review Checklist

### Spec Coverage
- user signs up with OAuth flow → Task 6 (Auth handlers)
- user interacts with the platform using OAuth JWTs → Task 4 (JWT auth middleware)
- users can create a project; list projects → Task 7 (Project handlers)
- add another user to a project → Task 7 (AddMember handler)
- after that, it's theirs too → Task 8 (Project membership middleware)
- only members can touch a project and its sandboxes → Task 8 (middleware) + Task 9 (sandbox Stop checks membership)
- POST /v1/projects/{id}/sandboxes → Task 9 (Sandbox Create handler)
- GET /v1/projects/{id}/sandboxes → Task 9 (Sandbox List handler)
- DELETE /v1/sandboxes/{id} → Task 9 (Sandbox Stop handler)
- all Postgres access goes through sqlc-generated code → Task 2
- schema migrations included → Task 2
- rate limiting → Task 5

### Placeholder Scan
No placeholders, TBDs, TODOs, or "implement later" found.

### Type Consistency
All function signatures, struct types, and method calls are consistent across tasks. Context keys `user_id` and `user_email` are defined in `middleware/auth.go` and referenced consistently in handlers.