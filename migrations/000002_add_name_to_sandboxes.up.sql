ALTER TABLE sandboxes ADD COLUMN name TEXT NOT NULL;

-- Sandbox names are mandatory (enforced at the API) and unique per project,
-- case-insensitively: "API" and "api" are the same name.
CREATE UNIQUE INDEX idx_sandboxes_project_name ON sandboxes (project_id, LOWER(name));