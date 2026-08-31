# Design & Implementation Decisions

What was built and why — every decision with the alternative it beat.
Companion: [IMPROVEMENTS.md](IMPROVEMENTS.md) covers what was deliberately
*not* built and how it would be.

## 1. System overview

A multi-tenant control-plane API for sandboxes: users sign in via Ory Hydra
(OAuth2 authorization-code flow) and call the API with the JWT; projects are
the tenancy unit; sandboxes are DB rows with a fake lifecycle (`running` ⇄
`stopped`, modeled by `stopped_at`) guarded by conditional updates. Every
write is rate-limited per user and quota-capped by the user's plan (owned
projects, running sandboxes).

Non-goals (deliberate): real orchestration, keyset pagination, refresh-token
sessions — IMPROVEMENTS.md.

## 2. Architecture & layering

```
main → server (Echo wiring)
         ├── middleware  (JWT auth, membership, rate limit, logging)
         ├── handler     (bind/validate → DTO, status mapping)
         └── service     (business rules, transactions, error semantics)
                └── db   (sqlc-generated; SQL is the source of truth)
```

**Three layers, strict dependency direction, no SQL outside
`internal/db/queries`.** Rejected: *fat handlers on sqlc* — membership,
quota and lifecycle rules would leak into every endpoint and be untestable
without a DB; *repository interfaces per service* — the generated `*Queries`
already is the narrow surface, a 1:1 wrapper adds nothing; *microservices* —
network failure modes with no scaling need at this size, and the layering
keeps extraction possible. Framework: **Echo v4** (the fixture's choice; gin
is equivalent, chi/net-http lack the built-in middleware).

**DI at the security seam only**: middleware depends on small interfaces
(`UserResolver`, `MembershipChecker`) that `*db.Queries` satisfies at wiring
time — the JWT and membership unit tests run without Postgres. Interfaces
without a second implementation are mild YAGNI; justified because middleware
is where behavioral testing matters most.

## 3. Data model

A single migration (`000001_init`) creates the complete schema: `users`
(with `plan_id`), `plans` (with the hobby/pro/ultimate seeds), `projects`,
`project_users`, `sandboxes`, plus every index
(`idx_project_users_user_id`, `idx_sandboxes_project_created_at`,
`idx_sandboxes_user_id`, `idx_sandboxes_user_running` (partial),
`idx_sandboxes_project_name`, the case-insensitive unique sandbox-name
index). No evolution migrations remain — the schema is the schema. All
access through sqlc.

**sqlc over ORMs/raw SQL.** Hand-written SQL, type-checked at generation,
query cost visible in review; cost: a `generate` step and no dynamic query
composition. GORM/ent hide the SQL that runs and drift from migrations; raw
`database/sql` moves renamed-column bugs from build time to runtime.

### 3.1 Identity: the OAuth subject, stored as the user's email

Login upserts the user keyed by the JWT's `sub` claim, stored in
`users.email` (`name` defaults to the subject too); every request resolves
via `GetUserByEmail(sub)`. Deliberately simple, and correct because the
fixture's Hydra sets `sub` = the email typed at the demo login page. The
upsert is atomic (`ON CONFLICT`), so concurrent callbacks cannot race. The
general-IdP case — opaque immutable subjects, email from `/userinfo`, a
dedicated `oauth_sub` column — is the first Identity item in
IMPROVEMENTS.md (an earlier implementation carried the column; removed as
machinery the fixture doesn't need).

### 3.2 `project_users`: composite PK + owner-only member addition

`(project_id, user_id)` PK makes "one membership per pair" a database fact;
`role CHECK IN ('owner','member')` keeps roles closed-set; reverse index
`idx_project_users_user_id (user_id, project_id)` serves user→projects
queries. **Roles are data and only owners add members** (the design spec's
`JWT + Owner` contract) — otherwise `role` is dead schema and "add your
friends" is a privilege-escalation foot-gun.

### 3.3 Sandbox state: `stopped_at`, no status column

State is the *presence* of `stopped_at`, not a status string — two columns
encoding one fact invite drift, and the timestamp records *when* for free;
`stopped_at IS NULL` doubles as the quota's "running" predicate and
motivates the partial index.

**Concurrency: guarded conditional updates, no version column.** Each
transition re-checks its precondition inside the UPDATE, under the row lock:
stop is guarded by `stopped_at IS NULL`, restart by `stopped_at IS NOT
NULL`. For a two-state, idempotent-shaped lifecycle this is sufficient —
competing writes converge instead of doubling. A `version` column /
`If-Match` scheme returns when transitions become non-idempotent or
side-effectful (a real orchestrator) — IMPROVEMENTS.md. `ON DELETE CASCADE`
keeps deletes single-statement.

### 3.4 `TEXT` UUID PKs

36 vs 16 bytes; at billions of rows the bloat hits every PK, FK and index.
Chosen because `database/sql` + `lib/pq` handle TEXT without type wrangling;
native `uuid` + `pgx` is the documented scale fix.

## 4. Authentication

### 4.1 OAuth2 authorization-code flow, confidential client

`client_secret_basic` against Hydra's token endpoint, per the fixture (PKCE
is the public-client upgrade path; implicit is deprecated; password grant
mishandles raw credentials). **State**: 32 bytes from `crypto/rand`, bound
to an `HttpOnly`+`Secure`+`SameSite=Lax` cookie, single-use, compared with
`subtle.ConstantTimeCompare`. Cookie-bound state over server-side storage
(no store dependency, no cleanup path — the OWASP stateless pattern) and
over JWT-signed state (same protection, more code). `Lax` because the
callback is a cross-site top-level navigation (`Strict` drops the cookie);
`Secure` works on localhost (trustworthy origin) — `curl` needs the cookie
passed manually, browsers don't. The callback surfaces Hydra's `error=`
redirects as explicit 400s and sets `Cache-Control: no-store` on the token
response (RFC 6749 §5.1).

### 4.2 Token validation (`internal/middleware/auth.go`)

1. **Signature** against Hydra's JWKS (`MicahParks/keyfunc`/`jwkset`)
2. **RS256 only** — blocks `none`/HS256/alg-confusion classes
3. **`exp` required** — a token without it would never expire
4. **Issuer pinned** to `HYDRA_PUBLIC_URL`
5. **`client_id` == configured client** — Hydra leaves `aud` empty unless
   audiences are explicitly requested (found by E2E: `aud`-based validation
   rejected *every* legitimate token), so `client_id` is the reliable
   per-client claim; blocks cross-client replay
6. **Subject → user** via `GetUserByEmail`; unknown subject → 401, fail
   closed

Rejected: per-request introspection (doubles latency, couples availability
to Hydra, the scale bottleneck); `aud` validation (correct in theory,
empirically wrong for Hydra — E2E-proven); HS256 (symmetric: any verifier
could also mint tokens).

**JWKS lifecycle**: 5-minute refresh; on refresh failure cached keys keep
serving and the failure is logged. Boot retries 5× (~10 s) so a slow Hydra
doesn't leave auth hard-down; after that, authed routes fail **closed**
(503).

### 4.3 `ParseUnverified` in the callback — accepted with guards

The callback token came from Hydra's token endpoint via a
client-authenticated server-to-server POST — re-verifying it would check
what Hydra just signed with keys from the same issuer. `ParseUnverified` +
explicit `iss`/`exp`/`client_id` checks catch a misconfigured provider
before user creation, at zero extra round-trips.

## 5. Subject → user resolution

`sub` → user on every request through a **60 s TTL, 10k-entry in-process
cache**; eviction drops expired entries, then the earliest-expiring
(approximate LRU) — the cache never silently disables itself at capacity
(the first implementation froze; caught in review). Staleness: a deleted
user keeps access ≤ 60 s, dwarfed by the token lifetime; any future
revocation feature must invalidate this cache. Rejected: no cache (identity
round-trip on every request), Redis (a hop plus an identity-critical store),
embedding the id in the token (Hydra mints them), singleflight (deferred).

## 6. Authorization

**Membership is enforced twice**: `ProjectMembership` middleware — one
`LEFT JOIN`, no row → 404, NULL role → 403 — and *inside the SQL* for every
sandbox query (`StopSandbox`, `RestartSandbox`, `GetSandboxByIDAndUser` all
join `project_users`), which is the only authorization for
`DELETE /v1/sandboxes/:id` (no project id in the path). Middleware gives
readable semantics; SQL gives the guarantee — a revocation between read and
write cannot resurrect access.

**404 vs 403**: missing project → 404, non-member → 403. An existence
oracle for authenticated callers, accepted because IDs are unguessable UUIDs
and client debugging matters. The original two queries (existence +
membership) collapsed into the single `LEFT JOIN` — one round-trip on the
hottest guarded path.

## 7. Rate limiting & quota

### 7.1 Fixed-window per-user limiter (Redis)

`ratelimit:user:<id>:<UTC-minute>`; `INCR` + `EXPIRE` on first hit; 429 with
`Retry-After` (from TTL) and `X-RateLimit-*` headers. `/auth/*` is
throttled per IP (unauthenticated, drives Hydra-side work). Accepted
weaknesses: `INCR`+`EXPIRE` are non-atomic (a crash between them leaks one
window key — a Lua script would fix it, declined for two plain commands);
fixed windows admit ~2× burst at boundaries (GCRA / sliding window is the
upgrade). **Fail-open vs fail-closed is a flag** (`RATE_LIMIT_FAIL_OPEN`,
default open): a Redis outage either removes protection exactly when retry
storms are likely, or turns the incident into a full outage — a judgment
call, not an accident, and `/health` reports Redis degraded either way.
Per-user keying so one noisy tenant cannot starve others.

### 7.2 Quotas: the plans table

The "plan with limits" is a `plans` table (seeds: `hobby` 5 projects / 3
running sandboxes, `pro` 25/20, `ultimate` 0 = unlimited) attached per user
(`users.plan_id`, default hobby). Limits scope over what the user *has*:

- **owned projects** (`role='owner'`) — membership in others' projects is
  free;
- **running sandboxes created by the user** (`sandboxes.user_id`,
  `stopped_at IS NULL`), across all projects.

**Enforcement** (check-then-create; 403 naming the plan and the limit):
sandbox `Create` *and* `Restart`, project `Create`. Restart is
quota-checked because a stopped sandbox does not count — restarting is
growth whenever new sandboxes were created since the stop; exempting it let
the running count exceed the cap without bound (found in review). The check
targets the **creator** (counts are by creator) — checking the actor
instead would let members trade their own headroom to push a capped creator
past the limit. Counts are **cap-bounded** (`LIMIT <plan cap>`): O(cap),
not O(user history); unlimited plans skip the count.

Soft by design: concurrent creates can overshoot by a few (bounded by the
rate limit; the overshoot persists while those sandboxes run); strict
enforcement means a per-user advisory lock or Redis counters reconciled to
Postgres — IMPROVEMENTS.md. Rejected: env-var cap (no domain grounding);
`INSERT..SELECT..WHERE count<limit` (still racy without serialization);
plan-per-project (contradicts "a user has a plan"); a management API (admin
scope beyond the README).

## 8. HTTP API surface

### 8.1 Endpoints

| Method | Path | Auth | Semantics |
|---|---|---|---|
| GET | `/health` | – | Liveness/readiness (see §10) |
| GET | `/auth/login` | IP-limited | Mint state, 302 → Hydra |
| GET | `/auth/callback` | IP-limited | Code→token, upsert user, return token JSON |
| GET/POST | `/v1/projects` | JWT + rate limit | List (paginated) / create (+owner membership, in one tx) |
| GET | `/v1/projects/:id` | JWT + member | Single project |
| POST | `/v1/projects/:id/members` | JWT + **owner** | Add member by email |
| GET/POST | `/v1/projects/:id/sandboxes` | JWT + member + rate limit | List / create (or restart when `sandbox_id` present) |
| DELETE | `/v1/sandboxes/:id` | JWT (SQL-enforced membership) | Stop a sandbox |

**Routing fallbacks (deliberate)**: auth middleware wraps the router's
fallback handlers, so unauthenticated callers get 401 for unknown routes
and wrong methods (no route enumeration); authenticated callers get 404
rather than RFC-405 — emitting 405 would require moving auth out of the
group, trading enumeration protection for cosmetic HTTP semantics.
`POST /sandboxes` doubling as restart via optional `sandbox_id` mirrors
E2B's create-or-restart ergonomics; `created` distinguishes 201 from 200
honestly.

### 8.2 Response contract

- **Explicit DTOs** (`internal/handler/dto.go`): snake_case fields,
  `stopped_at` as `*time.Time` (null while running). Models carry no JSON
  tags on purpose — the wire format must not couple to the schema
  (`sqlc emit_json_tags` couples it and still leaks `Null*` structs;
  returning models directly leaked PascalCase and internals — the bug this
  layer prevents).
- **Empty lists are `[]`, never `null`** (normalized in services).
- **Pagination**: `?limit` (default 50, cap 200), `?offset` → `{data,
  limit, offset, total, total_pages}`; invalid/negative params fall back to
  defaults; offset clamped to `math.MaxInt32` (a raw `int32` cast overflowed
  negative → Postgres 500). Offset pagination is the accepted scale
  limitation (O(n) deep pages, unstable under churn) — the keyset design is
  in IMPROVEMENTS.md.
- **Validation**: 1 MiB body limit; names/emails ≤ 255. Sandbox names are
  mandatory (trimmed; whitespace-only rejected) and **unique per project,
  case-insensitively** (`UNIQUE (project_id, LOWER(name))`) — Postgres
  `23505` → `ErrConflict` → 409, so the guarantee lives in the schema, not
  a check-then-lookup.

### 8.3 Status codes

`201` created / `200` read or state-transition / `204` stop / `400`
malformed input / `401` auth / `403` non-member, non-owner, quota / `404`
missing (cross-tenant lookups included — no existence leak) / `409`
duplicate member, re-stop, duplicate sandbox name / `429` rate limit / `503`
auth or limiter unavailable.

## 9. Error handling

**Sentinel errors** (`ErrNotFound`, `ErrConflict`, `ErrQuotaExceeded`)
mapped by handlers via `errors.Is`; SQLSTATEs are translated inside
services with named constants (`errCodeUniqueViolation` = 23505 →
`ErrConflict`, `errCodeForeignKeyViolation` = 23503 → `ErrNotFound`) so SQL
details never shape HTTP semantics. Rejected: `strings.Contains` on message
text (any DB error mentioning "not found" became a 404; duplicate members
500'd); per-endpoint error taxonomy (ceremony beyond the three sentinels).

**5xx never echo internals**: bodies become
`{"code":"INTERNAL_ERROR","message":"internal error"}`; the full error is
logged with request id, method and path. 4xx messages pass through — they
are client-facing by design. Multi-statement operations are transactional
(project+owner; member lookup+insert) — no ownerless projects, no raw FK
500s.

## 10. Operations

- **Migrations embedded** (`go:embed` + `migrate/source/iofs`):
  CWD-independent binary. Gap: golang-migrate takes no advisory lock — two
  instances can race at boot (run migrations as a deploy job).
- **HTTP timeouts** (header 5 s / read 15 s / write 30 s / idle 120 s): a
  default `http.Server` has none — slowloris clients pin connections
  forever.
- **Graceful shutdown**: SIGINT/SIGTERM → 10 s drain → cancel → close
  DB/Redis.
- **Pool bounds** (`DB_MAX_OPEN_CONNS=25`, idle 25, lifetime 1800 s):
  unbounded pools are the first failure at scale.
- **Health**: per-dependency statuses only — `database` (down → 503),
  `redis` (degraded → 200, matching fail-open), `auth` (JWKS loaded; down →
  503). Dial errors go to logs, not anonymous callers; the auth check exists
  because a stuck JWKS silently 503s every authed route.
- **Observability**: slog request logs (request id/method/path/status/
  duration; `/health` skipped) + error logs. OpenTelemetry removed
  deliberately; the RED-metrics/traces/counter plan is in IMPROVEMENTS.md.

## 11. Testing

- **Middleware unit tests, DB-free**: JWT rejection paths (alg/issuer/exp/
  client_id) with generated RSA keys; membership 401/403/404 branches;
  user-cache TTL/eviction/capacity; rate limiter 429 + `Retry-After` +
  fail-closed (skip-guarded on live Redis).
- **Handler tests**: OAuth state cookie lifecycle, `error=` surfacing.
- **Integration placeholders**: `t.Skip` stubs for the compose stack; live
  tests skip when dependencies are absent so `go test ./...` works anywhere.
- **E2E, shipped**: `scripts/e2e.sh` (43 checks — OAuth flow, authz matrix,
  lifecycle, sandbox-name rules) and `scripts/quota_e2e.sh` (18 checks —
  plan limits, slot freeing, restart-at-cap); throwaway per-run users,
  safe to re-run without resetting the database. This is what caught the
  bugs static review missed: the random-port binding, the `aud`-vs-Hydra
  incompatibility, and the restart quota bypass.

Rejected: testcontainers (worthwhile; deferred to stay dependency-light);
mock-heavy service tests (the behavior lives in SQL — mocks would test the
mock).

## 12. Summary of consciously accepted trade-offs

| Trade-off | Chosen | Given up | Where to upgrade |
|---|---|---|---|
| Non-atomic `INCR`+`EXPIRE` | Simplicity | One leaked key on crash-in-between | Lua script (was implemented, simplified) |
| Fixed-window limiting | Two commands, explainable | 2× burst at boundaries | GCRA / sliding window |
| Offset pagination | Simple + `total` | O(n) deep pages, unstable under churn | Keyset cursors |
| TEXT UUIDs | sqlc/`database/sql` simplicity | 36 vs 16 byte keys at scale | `uuid` + `pgx` |
| 60 s user-cache TTL | Huge DB-load cut | Revocation lag ≤ 60 s | Event-driven invalidation |
| Fail-open rate limiter (default) | Availability | Unprotected during Redis incidents | `RATE_LIMIT_FAIL_OPEN=false` |
| 404/403 distinction | Debuggability | Existence oracle for authed users | Uniform 404 |
| Quota count-then-insert | Simple, no locks | Contention overshoot (bounded by rate limit; persists while the sandboxes run) | Per-user advisory lock / Redis counters |
| No `Cookie` `__Host-` prefix / PKCE | Fixture compatibility | Marginal hardening left on table | IMPROVEMENTS.md |
| Demo secrets in config defaults | Runnable out of the box | Not production secrets hygiene | Env injection / vault |
| Email-keyed identity | Fixture simplicity (`sub` = email) | Breaks with opaque IdP subjects | `oauth_sub` column + `/userinfo` |