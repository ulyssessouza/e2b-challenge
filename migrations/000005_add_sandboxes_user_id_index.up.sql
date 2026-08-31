-- sandboxes.user_id has no index (Postgres does not index FK columns); any
-- user-scoped sandbox query or user deletion would sequentially scan.
CREATE INDEX idx_sandboxes_user_id ON sandboxes (user_id);