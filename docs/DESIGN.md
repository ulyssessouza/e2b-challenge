# Design & Implementation Decisions

This document explains what was built and why every decision was made the way
it was — each with the alternatives considered and the tradeoff accepted.
Companion doc: [IMPROVEMENTS.md](IMPROVEMENTS.md) covers what was deliberately
*not* built and how it would be.

---

## 1. System overview

A multi-tenant control-plane API for sandboxes:

- **Users** sign in through Ory Hydra (OAuth2 authorization-code flow) and call
  the API with the resulting JWT access token.
- **Projects** are the tenancy unit. Members can create sandboxes inside them.
- **Sandboxes** are DB records with a fake lifecycle (`running` ⇄ `stopped`,
  modeled by `stopped_at`), guarded by conditional updates.
- Every write is rate-limited (per user) and quota-capped by the user's plan (owned projects, running sandboxes).

Non-goals (documented, not accidental): real sandbox orchestration, keyset
pagination, refresh-token sessions. See IMPROVEMENTS.md.

---

## 2. Architecture & layering

```
cmd/main → server (Echo wiring)
              ├── middleware  (JWT auth, membership, rate limit, logging)
              ├── handler     (bind/validate → DTO, status mapping)
              └── service     (business rules, transactions, error semantics)
                     └── db   (sqlc-generated, SQL as source of truth)
```

**Decision: three layers with a strict dependency direction (handler → service
→ sqlc), no SQL outside `internal/db/queries`.**

| Alternative | Why rejected |
|---|---|
| Fat handlers calling sqlc directly | Fastest to write, but business rules (membership, quotas, versioning) leak into every endpoint; untestable without a DB; duplicable per endpoint. |
| Repository interfaces per service + mocks | With sqlc, the generated `*Queries` already *is* the narrow surface; wrapping every query in an interface adds a layer that mirrors 1:1 with no behavior change. Interfaces are introduced only at real seams (middleware, see below). |
| Microservices per domain | The spec is a single service; splitting adds network failure modes with no scaling need at this size. The layering keeps future extraction possible (services only depend on `db.Queries` + plain types). |

**Decision: dependency injection at middleware boundaries via small interfaces**
(`UserResolver`, `MembershipChecker` in `internal/middleware`). Middleware
cannot import concrete service logic; the concrete `*db.Queries` satisfies the
interfaces at wiring time in `server.New`. This made JWT/membership unit-testable
with trivial stubs — the tests in `auth_test.go`/`membership_test.go` run
without Postgres.

**Tradeoff accepted:** interfaces without a second implementation are a mild
YAGNI violation; justified because middleware sits at the security boundary
and is the code where behavioral testing matters most.

**Decision: Echo v4** — already the project framework; chosen over gin (similar)
and chi/net-http (less built-in: RequestID/Recover/BodyLimit middleware). No
strong opinion either way; the cost of the framework is low either way.

---

## 3. Data model

Schema (migrations 000001–000007): `users`, `plans`, `projects`,
`project_users`, `sandboxes` (indexes: `idx_project_users_user_id`,
`idx_sandboxes_project_created_at`, `idx_sandboxes_user_id`,
`idx_sandboxes_user_running`). All access through sqlc.

### 3.1 sqlc over ORMs / raw SQL

| Option | Tradeoff |
|---|---|
| **sqlc (chosen)** | Queries are hand-written SQL, type-checked at generation time; no runtime reflection, no N+1 surprises, query cost visible in review. Cost: a `sqlc generate` step; no dynamic query composition. |
| GORM/ent | Runtime query building hides the SQL that runs; migrations drift from models; harder to reason about per-query cost at scale. |
| Raw `database/sql` | No compile-time check of column/param ordering — a renamed column breaks at runtime, not at build. |

### 3.2 Identity: `oauth_sub`, not email (migration 000005)

**Decision:** users carry `oauth_sub TEXT UNIQUE NOT NULL` (the IdP subject);
JWTs resolve users via `oauth_sub`; `email`/`name` are profile data.

The original implementation conflated `sub` with `email`. That works only
because Hydra's demo login app sets `sub` = the typed email. With any real
provider, `sub` is opaque and stable — keying identity to email would (a)
create junk users, (b) break add-member-by-email lookups, (c) orphan a user's
projects if they change email upstream. The backfill (`oauth_sub = email`)
kept existing rows valid.

| Alternative | Why rejected |
|---|---|
| Keep email as identity | Identity keyed to a mutable, provider-churn-prone attribute. |
| Key by email *and* store sub but resolve by email | Same fragility, more columns. |
| Fetch email from `/userinfo` | Right move in production (noted in IMPROVEMENTS); unnecessary for the fixture where `sub` *is* the email — the column stays honest either way. |

### 3.3 `project_users` join table with composite PK

`(project_id, user_id)` PK enforces "one membership per pair" at the DB level;
`role CHECK (role IN ('owner','member'))` keeps roles closed-set. The reverse
index `idx_project_users_user_id (user_id, project_id)` (migration 000004)
serves user→projects queries; the PK serves project→member checks.

**Decision: roles are data (`owner`, `member`) with owner-only member
addition** — the design spec's `JWT + Owner` contract. Any member could add
members (friendlier for collaboration), but then `role` is dead schema and
"snap your own friends in" becomes a privilege-escalation foot-gun.

### 3.4 Sandbox state: `stopped_at TIMESTAMPTZ`, no status column

**Decision: state is the *presence* of `stopped_at`, not a status string.**

An earlier migration dropped a `status` column in favor of the timestamp.
Two columns encoding one fact invite drift (`status='stopped'`,
`stopped_at=NULL`); the timestamp also records *when* it stopped for free.
`stopped_at IS NULL` doubles as the "running" predicate for the quota count,
and partial index opportunities follow from the same predicate.

**Concurrency: guarded conditional updates, no version column.** Each
transition re-checks its precondition inside the UPDATE, under the row lock:
stop is guarded by `stopped_at IS NULL`, restart by `stopped_at IS NOT NULL`
(which also re-checks membership, so a revocation between read and write
cannot resurrect the sandbox). For this two-state, idempotent-shaped
lifecycle that is sufficient — competing writes converge instead of
doubling. A `version` column / `If-Match` scheme is deliberately omitted;
when transitions become non-idempotent or side-effectful (a real
orchestrator), reintroduce it — see IMPROVEMENTS.md.
`ON DELETE CASCADE` on `sandboxes.project_id`/`project_users` keeps deletes
one-statement at the DB level instead of app-level loops that can half-fail.

### 3.5 `TEXT` UUID PKs

36 bytes vs `uuid`'s 16; at billions of rows that bloats every PK, FK and
index. Chosen because `database/sql` + `lib/pq` handle `TEXT` without
type wrangling, and IDs never appear in a hot join-by-range pattern here.
Documented as a real scale fix in IMPROVEMENTS.md (native `uuid` + `pgx`).

---

## 4. Authentication

### 4.1 OAuth2 authorization-code flow, confidential client

`client_secret_basic` against Hydra's token endpoint, per the fixture.

| Alternative | Why rejected |
|---|---|
| PKCE | The right default for public clients; here the client is confidential with a secret. Listed in IMPROVEMENTS as defense-in-depth. |
| Implicit flow | Deprecated; tokens in URL fragments. |
| Resource-owner password | Requires handling raw credentials; forbidden by the spec's shape. |

**State parameter:** 32 bytes from `crypto/rand`, bound to an `HttpOnly` +
`Secure` + `SameSite=Lax` cookie, single-use (cleared on callback), compared
with `subtle.ConstantTimeCompare`.

| Alternative | Why rejected |
|---|---|
| Server-side state (Redis/DB) | Revocable and multi-instance-friendly, but adds a store dependency to login and a cleanup path; the cookie survives restarts and needs zero infrastructure. Cookie-bound state is the OWASP-recommended stateless pattern. |
| JWT-signed state | Equivalent protection; more code for no additional property here. |

`SameSite=Lax` (not `Strict`): the callback is a top-level cross-site
navigation from Hydra — `Strict` would drop the state cookie and break login.
`Secure` works on `localhost` because browsers treat it as a trustworthy
origin. (Note: `curl` does *not* send Secure cookies over plain HTTP — the
E2E script passes the cookie manually; browsers do not need that.)

The callback also handles Hydra's `error=`/`error_description=` redirects
(explicit 400 instead of a misleading "missing code") and sets
`Cache-Control: no-store` on the token response (RFC 6749 §5.1 — shared
caches must not retain an access token).

### 4.2 Token validation (`internal/middleware/auth.go`)

Every request verifies:

1. **Signature** against Hydra's JWKS (`MicahParks/keyfunc` + `jwkset`),
2. **Algorithm**: `WithValidMethods([]string{"RS256"})` — pins the compose
   fixture's documented strategy; blocks `none`/HS256/alg-confusion classes,
3. **Expiration required**: `WithExpirationRequired()` — a token without `exp`
   would otherwise never expire,
4. **Issuer pinned** to `HYDRA_PUBLIC_URL` (compose documents `iss` =
   `http://localhost:4444`),
5. **`client_id` claim == configured OAuth client** — Hydra leaves `aud` empty
   unless audiences are explicitly requested (found by E2E testing: `aud`-based
   validation rejected *every* legitimate token), so `client_id` is its
   reliable per-client identity claim. This keeps tokens minted for another
   first-party app of the same issuer from being replayed here.
6. **Subject resolution**: `sub` → internal user via `oauth_sub`; unknown
   subject → 401 (fail closed).

| Alternative | Why rejected |
|---|---|
| Token introspection (Hydra `/admin/oauth2/introspect`) per request | Central revocation + no key management, but doubles request latency, couples availability to Hydra, and is the documented bottleneck at scale. |
| `aud` validation | Correct in theory; empirically wrong for Hydra unless clients request audiences (E2E-proven). `client_id` gives the equivalent guarantee here. |
| HS256 shared secret | Single-tenant symmetric key: any verifier can also mint tokens; asymmetric RS256 keeps signing private to Hydra. |

**JWKS lifecycle** (`internal/jwks/provider.go`): HTTP storage with 5-minute
refresh; on refresh failure the previously cached keys keep serving (rotation
lags, availability doesn't). At boot the fetch is retried 5× with linear
backoff (~10 s total) — compose starts Hydra in parallel, and a startup race would
otherwise leave auth hard-down (every authed route 503s) until restart. If it
still fails, the service starts and authenticated routes fail **closed**
(503) rather than failing open.

### 4.3 Callback's `ParseUnverified` — accepted with guards

The callback token arrives from Hydra's token endpoint via a
client-authenticated server-to-server POST; verifying its signature again
would verify what Hydra just signed with the same keys we'd fetch from the
same issuer. `ParseUnverified` + explicit `iss`, `exp` and presence checks
catch a misconfigured provider before a user row is created, at zero extra
round-trips. Full verification would add a JWKS fetch on the login path for
no additional property in this topology.

---

## 5. Subject → user resolution at request time

**Decision:** JWT middleware resolves `sub` → internal user on every request,
through a **60-second TTL, 10,000-entry in-process cache with eviction**
(`CachedUserResolver`). Eviction: expired entries first, then the
earliest-expiring entry (approximate LRU without access tracking); at
capacity the cache keeps working instead of silently disabling itself (the
first implementation froze at capacity — caught in review).

| Alternative | Why rejected / deferred |
|---|---|
| No cache | One DB round-trip per authenticated request purely for identity; doubles DB load for hot users. |
| Redis-backed cache | Shared across instances, but adds a network hop to the hot path and Redis becomes identity-critical. |
| Embedding user id in the token | Not ours to mint — tokens come from Hydra. |
| `singleflight` stampede control | Worthwhile at high fan-out; deferred (IMPROVEMENTS.md). |

**Staleness contract:** a deleted/revoked user keeps access ≤ 60 s — bounded,
and dwarfed by the token's own lifetime. Any future revocation feature must
invalidate this cache; that coupling is documented here on purpose.

---

## 6. Authorization

**Decision: membership is enforced twice** — by the `ProjectMembership`
middleware (single `LEFT JOIN` query returning a nullable role; no row → 404,
row with NULL role → 403) and *inside the SQL* for cross-cutting routes:

- `StopSandbox` joins `project_users` in the UPDATE,
- `RestartSandbox` re-checks membership in the UPDATE (a revocation between
  the read and the write cannot resurrect the sandbox for a former member),
- `GetSandboxByIDAndUser` joins `project_users` for reads.

`DELETE /v1/sandboxes/:id` has no project id in the path, so middleware alone
cannot scope it — the SQL join is the authorization there. This is
defense-in-depth: middleware for readable 404/403 semantics, SQL for the
guarantee.

**Decision: 404 vs 403 distinction.** Missing project → 404; existing project
you don't belong to → 403. This is technically an existence oracle for
authenticated callers; accepted because project IDs are unguessable UUIDs and
the distinction makes client debugging tractable. (Returning 404 for both is
the stricter option; chosen debuggability over paranoia here.)

**Decision: single membership query instead of two.** The original code ran
`GetProjectByID` (result discarded) + `GetProjectMember` — two round-trips per
project-scoped request. The `LEFT JOIN` returns role nullability, halving the
per-request DB cost on the hottest guarded path.

---

## 7. Rate limiting & quota

### 7.1 Fixed-window per-user limiter (Redis)

Key: `ratelimit:user:<id>:<UTC-minute>`; `INCR`, `EXPIRE` on first hit;
429 with `Retry-After` (from `TTL`) and `X-RateLimit-Limit/Remaining` headers
so developers can see *why* they're throttled. `/auth/login` and
`/auth/callback` are throttled per IP (`ratelimit:ip:...`) because they are
unauthenticated and drive Hydra-side work (state minting, token exchange).

**Deliberately accepted weaknesses (documented in code):**

- **`INCR` + `EXPIRE` are not atomic** — a crash between them leaks one
  key without TTL (one user+window throttled until flush). A Lua script makes
  it atomic; rejected here for operational simplicity after weighing the
  worst case. (This was implemented atomically and simplified on request.)
- **Fixed windows admit ~2× burst at boundaries.** Sliding-window/GCRA fixes
  it (IMPROVEMENTS.md); the fixed window is two commands and trivially
  explainable.
- **Fail-open vs fail-closed is a config choice** (`RATE_LIMIT_FAIL_OPEN`,
  default *open*): during a Redis outage, fail-open keeps the demo usable but
  removes protection exactly when retry storms are likely; fail-closed
  protects the platform but turns a Redis incident into a full outage. There
  is no right answer — so it's a flag, not an accident, and `/health`
  reports Redis degraded so operators know which mode they're in.

Per-user keying (not global): the README's tenancy model is per-developer
limits; a global limiter would let one noisy tenant starve others.

### 7.2 Quotas: the plans table

The domain model's "plan with limits" is materialized as a `plans` table
(seed data: `hobby` 5 projects / 3 running sandboxes, `pro` 25/20,
`ultimate` unlimited), attached **per user** (`users.plan_id`, default
hobby). Limits scope over what a user *has*:

- **owned projects** (`project_users.role = 'owner'`) — membership in others'
  projects is free;
- **running sandboxes the user created** (`sandboxes.user_id`,
  `stopped_at IS NULL`), across all projects.

`Create` and `Restart` in the sandbox service, and `Create` in the project
service, check plan vs usage (check-then-create) and reject with 403 naming
the plan and the limit. **Restart is quota-checked too**: while a sandbox is
stopped it does not count toward the quota, so restarting is growth whenever
new sandboxes were created since the stop — exempting restart would let
running count exceed the cap without bound. 0 = unlimited. Enforcement is
soft (check-then-create): concurrent creates can overshoot the cap by a few,
bounded by the rate limit, and the overshoot persists while those sandboxes
run — strict enforcement (per-user advisory lock or Redis counters) is the
documented upgrade path. When a member restarts a sandbox someone else
created, the check targets the CREATOR's plan — the creator's count grows,
so checking the actor's instead would let members trade their own unused
headroom to push a capped creator past their limit. The bounded count
queries (`LIMIT <plan cap>`) keep the check O(cap), not O(user history).

| Alternative | Why rejected / deferred |
|---|---|
| Env-var hard cap (the previous implementation) | No domain grounding, no per-tier story, opaque to operators and users. |
| Atomic check+insert (`INSERT..SELECT..WHERE count<limit`) | Still racy without serialization; gnarly SQL for no real gain. |
| Redis atomic counters, reconciled to Postgres | Truly atomic but makes Redis a correctness dependency for data integrity; the plans design (IMPROVEMENTS.md) is where strict enforcement belongs. |
| Plan per project | Limits per project rather than per account; contradicts the "a user has a plan" model. |
| Plan management API | Auth scope (who is admin?) beyond the README; limits are editable seed rows. |

---

## 8. HTTP API surface

### 8.1 Endpoints

| Method | Path | Auth | Semantics |
|---|---|---|---|
| GET | `/health` | – | Liveness/readiness (see §10.2) |
| GET | `/auth/login` | IP-limited | Mint state, 302 → Hydra |
| GET | `/auth/callback` | IP-limited | Code→token, upsert user, return token JSON |
| GET/POST | `/v1/projects` | JWT + rate limit | List (paginated) / create (+owner membership, in one tx) |
| GET | `/v1/projects/:id` | JWT + member | Single project |
| POST | `/v1/projects/:id/members` | JWT + **owner** | Add member by email |
| GET/POST | `/v1/projects/:id/sandboxes` | JWT + member + rate limit | List / create (or restart when `sandbox_id` present) |
| DELETE | `/v1/sandboxes/:id` | JWT (SQL-enforced membership) | Stop a sandbox |

**Routing fallback behavior (deliberate):** authentication middleware wraps
the router's fallback handlers, so unauthenticated callers get 401 for
unknown routes and wrong methods (no route enumeration without a token);
authenticated callers get 404 rather than RFC-405 for unsupported methods —
Echo stamps the group middleware onto fallbacks, and emitting 405 would
require moving auth out of the group, which would trade away that
enumeration protection for cosmetic HTTP semantics.

`POST /sandboxes` doubling as restart via optional `sandbox_id` mirrors E2B's
real-world "create if absent, restart if present" ergonomics; `created`
distinguishes 201 (new resource) from 200 (state transition) honestly.

### 8.2 Response contract

- **Explicit DTOs** (`internal/handler/dto.go`): snake_case fields,
  `stopped_at` as `*time.Time` (JSON `null` while running). The DB models have
  no JSON tags on purpose — the API shape must not be coupled to the schema,
  and `sql.NullTime` would otherwise serialize as `{"Time":...,"Valid":false}`.

  | Alternative | Why rejected |
  |---|---|
  | `sqlc emit_json_tags` | Couples the wire format to column names and still leaks `Null*` structs. |
  | Return models directly | PascalCase keys + internal type leakage — the exact bug the DTO layer exists to prevent. |

- **Empty lists serialize as `[]`, never `null`** (normalized in the service
  layer) — clients can iterate without null checks.
- **Pagination:** `?limit` (default 50, cap 200), `?offset`; response carries
  `{data, limit, offset, total, total_pages}`. Invalid/negative params fall
  back to defaults silently; offsets are clamped to `math.MaxInt32` (a raw
  `int32` cast overflowed into negative offsets → Postgres 500). Offset
  pagination is the *accepted* scale limitation (O(n) pages, per-request
  `COUNT(*)`, unstable under concurrent inserts) — the keyset design that
  replaces it is written up in IMPROVEMENTS.md.
- **Validation:** 1 MiB body limit globally (`BodyLimit("1M")`), names/emails
  capped at 255 chars. Unbounded `TEXT` columns + unbounded bodies are an
  OOM lever; capping at the edge keeps the rule in one place.

### 8.3 Status codes

`201` created / `200` read or state-transition / `204` stop / `400` malformed
input / `401` auth / `403` non-member, non-owner, quota / `404` missing
(including cross-tenant lookups — no existence leak) / `409` duplicate member,
re-stop, guarded-write conflict / `429` rate limit / `503` auth or limiter unavailable.

---

## 9. Error handling

**Decision: sentinel errors in the service layer** (`ErrNotFound`,
`ErrConflict`, `ErrQuotaExceeded`), mapped to status codes by handlers via
`errors.Is`; unique-violation (`23505`) and FK-violation (`23503`) are
translated to `ErrConflict`/`ErrNotFound` inside services so SQL details
never shape HTTP semantics.

| Alternative | Why rejected |
|---|---|
| `strings.Contains(err.Error(), "not found")` | The original approach: any DB error mentioning "not found" became a 404; duplicate members 500'd; semantics lived in message text. Fragile by construction. |
| Per-endpoint error types | More precise, but a struct taxonomy per service is ceremony without callers that need to distinguish beyond the three sentinels. |

**5xx responses never echo internals.** `HTTPErrorHandler` replaces any
≥500 body with `{"code":"INTERNAL_ERROR","message":"internal error"}` and
logs the full error with `request_id`, method, path via `slog`. 4xx messages
pass through — they're client-facing by design. The failure mode this
prevents: `err.Error()` from pq leaks SQL, constraints and hostnames.

**Multi-statement operations are transactional** (project+owner insert;
member lookup+insert) so partial failure can't leave an ownerless project or
surface a raw FK error as 500. `defer tx.Rollback()` + explicit `Commit` is
the standard Go pattern; the deferred rollback is a no-op after commit.

---

## 10. Operations

### 10.1 Process lifecycle

- **Migrations embedded** (`go:embed` + `migrate/source/iofs`): the binary is
  self-contained and CWD-independent. Alternative — `file://` paths — breaks
  when the binary isn't launched from the repo root. Known gap: golang-migrate
  takes no advisory lock, so two instances booting together can race
  (documented; deployment jobs should run migrations).
- **HTTP server timeouts** (`ReadHeaderTimeout` 5 s, read 15 s, write 30 s,
  idle 120 s): a default `http.Server` has *zero* timeouts — slowloris
  clients pin connections forever. `WriteTimeout` bounds any handler,
  including slow DB queries.
- **Graceful shutdown**: SIGINT/SIGTERM → `Shutdown` with a 10 s drain →
  cancel contexts → close DB/Redis. In-flight requests drain instead of
  dying mid-write.
- **DB pool bounds**: `DB_MAX_OPEN_CONNS=25`, `DB_MAX_IDLE_CONNS=25`,
  `DB_CONN_MAX_LIFETIME_SECS=1800`. Without bounds, each instance opens
  unbounded Postgres connections under load — the first failure at scale.
  Idle conn rotation avoids stale-connect errors after failovers.

### 10.2 Health

`/health` returns per-dependency status: `database` (down → 503),
`redis` (degraded → still 200, matching the limiter's fail-open default),
`auth` (JWKS loaded; down → 503). Bodies carry *statuses only* — dial errors
(hostnames, ports) go to logs, not to anonymous callers. The auth check
exists because JWKS-load failure silently 503s *every* authed route; without
it the health check would lie.

### 10.3 Observability

Structured `slog` request logs (request id, method, path, status, duration;
`/health` skipped) + error logs with request ids. OpenTelemetry was removed
from this submission deliberately; the replacement plan (RED metrics, traces,
rate-limit rejection counters) is in IMPROVEMENTS.md rather than half-wired
here.

---

## 11. Testing

- **Middleware unit tests** (no DB): JWT alg/issuer/expiration/client_id
  rejection paths with generated RSA keys; membership 401/403/404 branches;
  user-cache TTL/eviction/capacity; rate limiter 429 + `Retry-After` +
  fail-closed (skip-guarded on live Redis).
- **Handler tests**: OAuth state cookie lifecycle, `error=` surfacing.
- **Integration placeholders**: service/handler suites are `t.Skip` stubs
  meant for the compose stack; live-service tests skip (not fail) when
  dependencies are absent so `go test ./...` works anywhere.
- **End-to-end**: the full browser flow (emulated over HTTP with Hydra's
  admin accept endpoints) plus the complete authz/lifecycle/limits matrix,
  exercised against the live stack via the shipped scripts
  (`scripts/e2e.sh` — 34 checks; `scripts/quota_e2e.sh` — 16 plan-limit
  checks, run after it on a fresh database). This is what caught the two
  bugs static review missed (empty-`Addr` random port binding; `aud`
  validation incompatible with Hydra's empty-audience tokens) and the
  restart-quota bypass found in final review.

| Alternative | Why rejected / deferred |
|---|---|
| testcontainers for integration tests | Real DB coverage in CI without compose; worthwhile, deferred to keep the submission dependency-light. |
| Mock-heavy service tests | The interesting behavior (guarded transitions, membership joins, quotas) lives in SQL — mocks would test the mock. The E2E script covers those paths for real. |

---

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