# Improvements Not Implemented

The implementation favors correctness and clarity over scale. Below is what
would change to support E2B's real workload — millions of users and billions
of sandboxes — roughly ordered by impact.

## Pagination: offset → keyset cursors

`GET /v1/projects` and `GET /v1/projects/:id/sandboxes` use `LIMIT/OFFSET`
plus a `COUNT(*)` per request. At billions of rows:

- `OFFSET n` makes the database walk and discard `n` rows — page 10,000 of a
  billion-row list costs 10,000 rows of I/O.
- `COUNT(*)` per list request is O(table/project size) on every call.
- Concurrent inserts shift offsets between pages (skipped/duplicated items).

**Design**: keyset pagination on `(created_at, id)` — both covered by the
existing index — with an opaque cursor token:

```sql
SELECT * FROM sandboxes
WHERE project_id = $1 AND (created_at, id) < ($2, $3)
ORDER BY created_at DESC, id DESC
LIMIT $4 + 1;   -- one extra row yields has_more
```

Response: `{data, next_cursor, has_more}`; drop `total` (or serve it from an
approximate counter).

## Data tier at billions of rows

- **UUID primary keys**: `TEXT` PKs (36 bytes) bloat every PK, FK, and index;
  use native `uuid` columns (16 bytes) with `pgx`.
- **Partitioning**: range-partition `sandboxes` by `created_at` (or hash by
  `project_id`) so pruning keeps hot queries on recent partitions.
- **Connection pooling**: PgBouncer in transaction mode between the app and
  Postgres; the per-process pool limits recently added here remain the first
  line of defense.
- **Read replicas**: list endpoints are read-heavy and lag-tolerant; route
  them to replicas.

## Quotas and "plans"

The basics exist: a `plans` table (hobby / pro / ultimate seeds) attached
per user, with owned-project and running-sandbox limits enforced on create
(check-then-create, so concurrent creates can overshoot by a few). What a
real system adds on top:

- **Plan management**: endpoints/flow to change a user's plan (upgrades,
  billing) — currently limits are edited as seed rows in SQL.
- **Strict atomicity**: a Redis counter for the hot path (atomic, like the
  rate limiter), reconciled asynchronously against Postgres to heal drift
  from crashes.
- **More limit kinds**: e.g. `max_sandboxes_created_per_day`, storage/API
  budgets per tier.
- **Rejection ergonomics**: typed `QUOTA_EXCEEDED` error code and
  `Retry-After` on 403s.

## Rate limiting → sliding window

The fixed-minute window admits up to 2× the limit across a boundary. GCRA or
a sliding-window Lua script smooths this without extra round-trips.

## Identity

- Identity is keyed by `users.email`, seeded from the JWT subject — correct
  here because the fixture's Hydra sets `sub` = the typed email. A real IdP
  issues opaque, immutable subjects, so production adds a dedicated
  `oauth_sub` column (unique), resolves JWTs by it, and takes `email` from
  the ID token / `/userinfo` — which also fixes the edge where two subjects
  share an email (the `users.email UNIQUE` constraint would 500 the second
  login today). Minor: the login upsert's no-op `DO UPDATE` (needed to get
  the row back from `RETURNING`) could become `DO NOTHING` + re-select.
- Add PKCE to the authorization-code flow (cheap, protects public clients).
- Refresh-token rotation with server-side storage for session revocation.

## Sandbox lifecycle as a system

Sandboxes here are DB rows with a fake lifecycle, guarded by conditional
updates — sufficient for this two-state lifecycle. Once transitions become
non-idempotent or carry side effects (a real orchestrator), reintroduce
optimistic locking (a `version` column / HTTP `ETag` / `If-Match`): it turns
a stale decision into an explicit 409-and-retry instead of silent
convergence. Beyond that:

- an event/outbox pattern (`sandbox.created/started/stopped`) publishing to a
  stream for consumers (webhooks, billing, metrics),
- a reconciler that marks sandboxes whose backing container died,
- eventual-consistency between the API record and the real sandbox.

## Observability

OpenTelemetry was deliberately removed for this assignment; at production
scale it would come back as:

- RED metrics per endpoint (rate, errors, duration) with `request_id` and
  `user_id` labels,
- distributed traces spanning middleware → service → SQL,
- rate-limit rejection and quota-exhaustion counters (they are the earliest
  signal of a runaway agent loop).

## Operational

- Run migrations as a deploy job (golang-migrate lacks an advisory lock; two
  racing instances can dirty the database on first boot).
- Ship a Dockerfile; the compose stack currently only builds dependencies.
- Alert on `rate limiter: redis unreachable` warnings — they mean the
  platform is unprotected (fail-open) or refusing traffic (fail-closed).
