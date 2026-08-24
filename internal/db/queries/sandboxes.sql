-- name: CreateSandbox :one
INSERT INTO sandboxes (project_id, user_id) VALUES ($1, $2) RETURNING *;

-- name: GetSandboxByID :one
SELECT * FROM sandboxes WHERE id = $1 LIMIT 1;

-- name: ListSandboxesByProject :many
SELECT * FROM sandboxes WHERE project_id = $1 ORDER BY created_at DESC;

-- name: UpdateSandboxStatus :exec
UPDATE sandboxes SET status = $2, stopped_at = now() WHERE id = $1;