# Sandbox API Service

## The Problem

Every E2B sandbox is created, monitored, and killed through our public API.
Developers authenticate with Bearer tokens, and behind every request is a user, a
project, and a plan with limits. When we get this layer wrong, one runaway agent
loop can hammer the platform or developers can't tell why their requests are
failing.

**Your job**: Build a small version of E2B's public API service — the auth, projects,
and rate-limiting/quota system that sits in front of the sandboxes.

**The domain model**:
  - **Users** sign in with OAuth and get a JWT
  - **Projects** are the tenancy unit — a user can have many projects, and a project
    can be shared by many users
  - **Sandboxes** always run inside a project

## Getting Started

At the end of this document you are provided `docker compose up` with **PostgreSQL**,
**Redis**, and **Ory Hydra** — a pre-configured OAuth2 provider with a demo login
page and a test user, so you don't need any external accounts

**Before building**:
  - Bring the compose stack up and log in through the demo Hydra login page
  - Find the pre-configured OAuth client credentials and redirect URI in the
    compose file

## Running & verifying

```sh
docker compose up -d --wait   # Postgres, Redis, Hydra (fixture)
go run .                      # starts on :8080, runs migrations, logs in via demo page
```

Log in at `http://localhost:8080/auth/login` (demo user `foo@bar.com` /
`foobar`), then call the API with the returned access token.

End-to-end suites (require the running server; safe to re-run — each run
uses throwaway users):

```sh
scripts/e2e.sh            # 43 checks: OAuth flow, authz matrix, lifecycle, name rules
scripts/quota_e2e.sh      # 18 checks: plan limits, slot freeing, restart-at-cap
```

Both scripts exit non-zero on failure. Design decisions and tradeoffs:
[`docs/DESIGN.md`](docs/DESIGN.md).

## Usage examples (curl)

Get a token first: open [`/auth/login`](http://localhost:8080/auth/login) in a
browser, sign in with `foo@bar.com` / `foobar`, and copy the `access_token`
from the JSON response. Then (examples use [jq](https://jqlang.github.io/jq/)):

```sh
export TOKEN=…   # the access_token you copied
```

### Happy path: create a project, add a sandbox, stop it

```sh
# Create a project → 201
curl -s -X POST http://localhost:8080/v1/projects \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo"}'

PROJECT_ID=$(curl -s http://localhost:8080/v1/projects \
  -H "Authorization: Bearer $TOKEN" | jq -r '.data[0].id')

# Create a sandbox inside it → 201 (stopped_at: null means running)
curl -s -X POST http://localhost:8080/v1/projects/$PROJECT_ID/sandboxes \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-sandbox"}'

SANDBOX_ID=$(curl -s http://localhost:8080/v1/projects/$PROJECT_ID/sandboxes \
  -H "Authorization: Bearer $TOKEN" | jq -r '.data[0].id')

# Stop it → 204
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE \
  http://localhost:8080/v1/sandboxes/$SANDBOX_ID \
  -H "Authorization: Bearer $TOKEN"
```

### Conflict: stop it again → 409

```sh
curl -s -X DELETE http://localhost:8080/v1/sandboxes/$SANDBOX_ID \
  -H "Authorization: Bearer $TOKEN"
# {"code":"CONFLICT","message":"conflict: sandbox already stopped"}
```

## What to Build

Build a Go service that exposes:
  - user signs up with OAuth flow
  - user interacts with the platform using OAuth JWTs
  - users can create a project; list projects - add another user to a project - after
    that, it's theirs too
  - only members can touch a project and its sandboxes
  - `POST /v1/projects/{id}/sandboxes` — creates a sandbox record (fake lifecycle is
  fine: running , stopped )
  - `GET /v1/projects/{id}/sandboxes` — lists the project's sandboxes
  - `DELETE /v1/sandboxes/{id}` — stops one
  - all Postgres access goes through sqlc-generated code
  - schema migrations included — users, projects, sandboxes, etc ...
  
From an architectural perspective, we'll evaluate the design decisions throughout
your submission. Ask yourself: Would this architecture support millions of users
and billions of sandboxes?

We don't expect the implementation itself to scale to that level, but the underlying
architecture should.

If you have ideas for improvements that you didn't have time to implement, feel
free to describe them in a short Markdown document outlining your proposed
solution.

## Design documentation

- [`docs/DESIGN.md`](docs/DESIGN.md) — what was implemented, every decision and
  the tradeoffs against the alternatives considered
- [`docs/IMPROVEMENTS.md`](docs/IMPROVEMENTS.md) — deliberately unimplemented
  ideas and how they would be built
- `docs/superpowers/` — historical planning artifacts from the build process
  (superseded by DESIGN.md where they disagree)