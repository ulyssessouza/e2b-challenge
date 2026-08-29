CREATE TABLE plans (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    max_projects          INTEGER NOT NULL,
    max_running_sandboxes INTEGER NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO plans (id, name, max_projects, max_running_sandboxes) VALUES
    ('plan-hobby',    'hobby',    5,  3),
    ('plan-pro',      'pro',     25, 20),
    ('plan-ultimate', 'ultimate', 0,  0);

ALTER TABLE users ADD COLUMN plan_id TEXT NOT NULL DEFAULT 'plan-hobby' REFERENCES plans(id);