CREATE TABLE plans (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    max_projects          INTEGER NOT NULL CHECK (max_projects >= 0),
    max_running_sandboxes INTEGER NOT NULL CHECK (max_running_sandboxes >= 0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO plans (id, name, max_projects, max_running_sandboxes) VALUES
    ('plan-hobby',    'hobby',    5,  3),
    ('plan-pro',      'pro',     25, 20),
    ('plan-ultimate', 'ultimate', 0,  0);

CREATE TABLE users (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    plan_id    TEXT NOT NULL DEFAULT 'plan-hobby' REFERENCES plans(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Project names are unique per OWNER, case-insensitively: "API" and "api"
-- are the same name, and two owners may each have their own "demo".
CREATE UNIQUE INDEX idx_projects_owner_name ON projects (owner_id, LOWER(name));

CREATE TABLE project_users (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, user_id)
);

-- project_users is the access list; ownership lives on projects.owner_id.
-- The PK is (project_id, user_id); user-scoped lookups need the reverse
-- ordering to avoid sequential scans when listing a user's projects.
CREATE INDEX idx_project_users_user_id ON project_users (user_id, project_id);

CREATE TABLE sandboxes (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at TIMESTAMPTZ
);

-- State is the presence of stopped_at: NULL means running (no status column).

-- Sandbox names are mandatory (enforced at the API) and unique per project,
-- case-insensitively: "API" and "api" are the same name.
CREATE UNIQUE INDEX idx_sandboxes_project_name ON sandboxes (project_id, LOWER(name));

-- Sandboxes are always listed/counted per project and ordered by creation time.
CREATE INDEX idx_sandboxes_project_created_at ON sandboxes (project_id, created_at DESC);

-- The plan quota counts only running sandboxes; the partial index keeps that
-- count proportional to running, not to all sandboxes the user ever had.
CREATE INDEX idx_sandboxes_user_running ON sandboxes (user_id) WHERE stopped_at IS NULL;

-- Any user-scoped sandbox query or user deletion would otherwise sequentially scan.
CREATE INDEX idx_sandboxes_user_id ON sandboxes (user_id);