-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: UpsertUserByEmail :one
-- The no-op DO UPDATE guarantees a row is always returned: first login
-- inserts, later logins return the existing row atomically (no
-- check-then-insert race between concurrent callbacks).
INSERT INTO users (email, name) VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET email = users.email
RETURNING *;