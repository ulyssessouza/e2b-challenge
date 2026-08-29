# Plans — Design

**Date:** 2026-08-29
**Status:** Approved (replaces the removed `MAX_RUNNING_SANDBOXES_PER_PROJECT` env cap)

## Problem

The README's domain model says every request has "a user, a project, and a
plan with limits" and the service is "the auth, projects, and
rate-limiting/quota system". Rate limiting bounds requests per minute; it does
not bound the actual resource (running sandboxes) or project sprawl. The
previous env-var cap (`MAX_RUNNING_SANDBOXES_PER_PROJECT`) was an invented,
per-project hard limit with no domain grounding.

## Decision

A `plans` table, attached **per user**. Limits scope over what a user *has*:

- **Projects**: projects the user **owns** (`project_users.role = 'owner'`).
  Being a member of someone else's project is free.
- **Running sandboxes**: sandboxes the user **created**
  (`sandboxes.user_id`) with `stopped_at IS NULL`, across all projects.

## Schema (migration 000007_add_plans)

```sql
CREATE TABLE plans (
    id                    TEXT PRIMARY KEY,   -- literal: 'plan-hobby', ...
    name                  TEXT NOT NULL UNIQUE,
    max_projects          INTEGER NOT NULL,   -- 0 = unlimited
    max_running_sandboxes INTEGER NOT NULL,   -- 0 = unlimited
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO plans (id, name, max_projects, max_running_sandboxes) VALUES
    ('plan-hobby',    'hobby',    5,  3),
    ('plan-pro',      'pro',     25, 20),
    ('plan-ultimate', 'ultimate', 0,  0);
ALTER TABLE users ADD COLUMN plan_id TEXT NOT NULL DEFAULT 'plan-hobby' REFERENCES plans(id);
```

The literal `plan-hobby` id lets the column default assign new users without
subselects. Plans are **seed data**: changing limits = editing the row in SQL.
No management API (out of scope; see IMPROVEMENTS.md).

## Seeded tiers

| Plan | Max owned projects | Max running sandboxes |
|---|---|---|
| `hobby` | 5 | 3 |
| `pro` | 25 | 20 |
| `ultimate` | unlimited (0) | unlimited (0) |

## Enforcement

Check-then-create in the services (soft, documented TOCTOU — bounded by
concurrency, re-evaluated per request):

- `ProjectService.Create`: `GetUserPlan` + `CountProjectsOwnedByUser`; reject
  with `ErrQuotaExceeded` when `max_projects > 0 && owned >= max`.
- `SandboxService.Create`: `GetUserPlan` + `CountRunningSandboxesByUser`;
  reject when `max_running_sandboxes > 0 && running >= max`.
- **Restart is quota-checked**: while a sandbox is stopped it does not count
  toward the quota, so restarting is growth whenever new sandboxes were
  created since the stop (otherwise running count could exceed the cap
  without bound). Restoring a sandbox the user just stopped still works —
  the freed slot makes room again.
- Rejections are 403 with the plan name and limit in the message ("plan
  'hobby' allows 3 running sandboxes") so developers can tell *why*.

`ErrQuotaExceeded` → 403 mapping already exists in the sandbox handler; added
to the project handler. The env cap, its config field, test, and service
constructor parameter are removed.

## Alternatives rejected

- **Atomic check+insert** (`INSERT ... SELECT ... WHERE count < limit`):
  still racy without serialization; gnarly SQL for no real gain.
- **Redis atomic counters**: truly atomic, but makes Redis a correctness
  dependency for data integrity; needs reconciliation. Overkill here.
- **Plan per project**: limits per project instead of per user — contradicts
  the confirmed per-user scoping.
- **Management API**: auth scope (who is admin?) beyond the README.

## Testing

- Unit: config test loses the env field (no service-level unit tests without
  a DB; see existing skipped integration stubs).
- E2E (live stack): 4th running sandbox → 403 with plan message; 6th owned
  project → 403; stop frees a slot (create succeeds again); restart when at
  cap → 403 (the slot must be freed first); all prior lifecycle checks stay
  green.
