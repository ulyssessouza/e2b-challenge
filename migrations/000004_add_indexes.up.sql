-- project_users PK is (project_id, user_id); user-scoped lookups need the
-- reverse ordering to avoid sequential scans when listing a user's projects.
CREATE INDEX idx_project_users_user_id ON project_users (user_id, project_id);

-- Sandboxes are always listed/counted per project and ordered by creation time.
CREATE INDEX idx_sandboxes_project_created_at ON sandboxes (project_id, created_at DESC);