# Contributing

## Prerequisites

- [mise](https://mise.jdx.dev) — installs Go and sqlc automatically
- Docker + Docker Compose

## Inner-loop development

```sh
# Start dependencies (Postgres, Redis, Hydra)
mise run up

# Run the server
mise run dev

# After changing SQL queries, regenerate Go code
mise run db-gen

# Run tests
mise run test

# Full check (tidy → vet → build → test)
mise run check
```

`mise run dev` is equivalent to `docker compose up -d --wait && go run .`.

## Example: modifying a query

```sh
# 1. Edit a .sql file in internal/db/queries/
# 2. Regenerate the Go code
mise run db-gen
# 3. Update the service/handler that calls it
# 4. Run tests
mise run test
```

## Available tasks

| Task | Runs |
|------|------|
| `mise run up` | `docker compose up -d --wait` |
| `mise run down` | `docker compose down` |
| `mise run reset` | Full reset of all containers + volumes |
| `mise run db-gen` | `sqlc generate` |
| `mise run build` | `go build ./...` |
| `mise run test` | `go test ./... -count=1` |
| `mise run vet` | `go vet ./...` |
| `mise run check` | tidy → vet → build → test |
| `mise run dev` | up + run |

## Project layout

```
├── main.go              # entrypoint
├── compose.yml          # Postgres, Redis, Hydra
├── migrations/          # database migrations (golang-migrate)
├── sqlc.yaml            # sqlc config
├── internal/
│   ├── config/          # environment-based config
│   ├── db/              # sqlc-generated code
│   │   └── queries/     # hand-written SQL
│   ├── handler/         # HTTP handlers
│   ├── middleware/       # Echo middleware (auth, rate-limit)
│   ├── service/         # business logic
│   ├── server/          # Echo server setup
│   ├── jwks/            # JWKS key provider
│   └── pagination/      # offset pagination envelope
├── scripts/             # e2e.sh, quota_e2e.sh (live end-to-end suites)
└── docs/                # DESIGN.md, IMPROVEMENTS.md, specs
```

## OAuth test credentials

| | |
|---|---|
| Client ID | `e2b-assignment` |
| Client secret | `e2b-assignment-secret` |
| Redirect URI | `http://localhost:8080/auth/callback` |
| Test user | `foo@bar.com` / `foobar` |
| Hydra public | `http://localhost:4444` |
| JWKS endpoint | `http://localhost:4444/.well-known/jwks.json` |