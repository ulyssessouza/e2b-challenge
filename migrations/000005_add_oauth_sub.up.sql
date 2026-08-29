ALTER TABLE users ADD COLUMN oauth_sub TEXT;
UPDATE users SET oauth_sub = email WHERE oauth_sub IS NULL;
ALTER TABLE users ALTER COLUMN oauth_sub SET NOT NULL;
CREATE UNIQUE INDEX idx_users_oauth_sub ON users (oauth_sub);