-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: GetUserByOAuthSub :one
SELECT * FROM users WHERE oauth_sub = $1 LIMIT 1;

-- name: UpsertUserByOAuthSub :one
INSERT INTO users (oauth_sub, email, name) VALUES ($1, $2, $3)
ON CONFLICT (oauth_sub) DO UPDATE SET oauth_sub = users.oauth_sub
RETURNING *;