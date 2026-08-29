# E2B Sandbox API Service — Design Spec

## Architecture

```
Client → Echo Router → Middleware Stack → Handlers → Service Layer → sqlc → PostgreSQL
                                                          ↕
                                                       Redis (rate limit + JWKS cache)
```

Layered monolith with clean boundaries. Each layer depends only on the layer below it.

## Middleware Stack (order matters)

1. **Recover / Logger** — echo default
2. **JWTAuth** — fetches JWKS from Hydra `/.well-known/jwks.json`, caches in Redis (TTL 5 min), validates `Authorization: Bearer <jwt>` on every protected route
3. **RateLimiter** — sliding window per user in Redis, configurable limits
4. **ProjectMembership** — for `/v1/projects/:id/*` routes, verifies caller is a member of the project

Unauthenticated routes: `/auth/login`, `/auth/callback`, health check.

## Database Schema (Postgres via sqlc)

### migrations/

```sql
CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE project_role AS ENUM ('owner', 'member');

CREATE TABLE project_users (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       project_role NOT NULL DEFAULT 'member',
    PRIMARY KEY (project_id, user_id)
);

CREATE TYPE sandbox_status AS ENUM ('running', 'stopped');

CREATE TABLE sandboxes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id),
    status     sandbox_status NOT NULL DEFAULT 'running',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at TIMESTAMPTZ
);
```

All queries live in `internal/db/queries/*.sql` and are processed by sqlc into `internal/db/`.

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/auth/login` | No | Redirect to Hydra OAuth authorize |
| GET | `/auth/callback` | No | Exchange auth code for tokens, return access token |
| GET | `/v1/projects` | JWT | List user's projects |
| POST | `/v1/projects` | JWT | Create project |
| GET | `/v1/projects/:id` | JWT + Member | Get project details |
| POST | `/v1/projects/:id/members` | JWT + Owner | Add user to project |
| GET | `/v1/projects/:id/sandboxes` | JWT + Member | List sandboxes in project |
| POST | `/v1/projects/:id/sandboxes` | JWT + Member | Create sandbox (fake lifecycle) |
| DELETE | `/v1/sandboxes/:id` | JWT + Member | Stop sandbox |

### Auth Flow

1. `GET /auth/login` returns HTML with redirect link to Hydra's `/oauth2/auth?client_id=e2b-assignment&response_type=code&scope=openid&redirect_uri=http://localhost:8080/auth/callback`
2. User logs in at Hydra's demo login page (foo@bar.com / foobar) and consents
3. Hydra redirects to `GET /auth/callback?code=<auth_code>`
4. Server POSTs to Hydra's `/oauth2/token` with the code + client credentials
5. Returns the access token as JSON `{"access_token": "..."}`
6. Client uses `Authorization: Bearer <token>` for all subsequent API calls
7. Server validates JWT against cached JWKS — extracts `sub` (OAuth subject), looks up or creates local user by `sub` (which is the user's email from Hydra)

### Rate Limiting

- Sliding window counter per user in Redis
- Default: 1000 req/min per user
- Sandbox creation: 100 req/min per project
- Returns `429 Too Many Requests` with `Retry-After` header
- Configurable via environment variables

## Package Layout

```
.
├── main.go
├── go.mod / go.sum
├── compose.yml
├── Dockerfile
├── migrations/
│   └── 000001_init.up.sql
│   └── 000001_init.down.sql
├── internal/
│   ├── config/       — env-based configuration
│   ├── server/       — Echo setup, middleware, routes
│   ├── handler/      — HTTP handlers
│   ├── service/      — business logic (projects, sandboxes, auth)
│   ├── middleware/    — JWT auth, rate limiter, project membership
│   ├── db/           — sqlc-generated code
│   │   ├── queries/  — *.sql files for sqlc
│   │   ├── models.go
│   │   └── *.sql.go
│   └── jwks/         — JWKS fetching and caching
└── sqlc.yaml
```

## Scaling Considerations

- Stateless JWT auth → any instance serves any request; horizontal scale via load balancer
- Redis is the rate-limiting bottleneck → could shard by `user_id % N` at scale
- sqlc layer makes DB access type-safe and query costs visible
- Service layer has clear interfaces → projects, sandboxes, auth can become independent services if monolith grows
- All mutable operations go through service layer; middleware is read-only (auth, rate limit, membership check)

## What's Not Included (Future Improvements)

Implemented since this spec was written: offset pagination on list endpoints,
structured error responses, request logging, rate-limit headers, sandbox
quota per project, and OAuth `sub`-keyed identity (see docs/IMPROVEMENTS.md
for what remains).

- Refresh token handling (tokens expire; need rotation and storage)
- Keyset (cursor) pagination for very large collections
- Soft-delete / audit logging for sandboxes
- OpenAPI / swagger spec generation
- Metrics / tracing instrumentation