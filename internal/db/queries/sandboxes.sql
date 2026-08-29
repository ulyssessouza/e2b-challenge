-- name: CreateSandbox :one
INSERT INTO sandboxes (project_id, user_id, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetSandboxByIDAndUser :one
SELECT s.* FROM sandboxes s
JOIN project_users pu ON pu.project_id = s.project_id
WHERE s.id = $1 AND pu.user_id = $2;

-- name: ListSandboxesByProject :many
SELECT * FROM sandboxes WHERE project_id = $1 ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountSandboxesByProject :one
SELECT COUNT(*) FROM sandboxes WHERE project_id = $1;

-- name: CountRunningSandboxesByProject :one
SELECT COUNT(*) FROM sandboxes WHERE project_id = $1 AND stopped_at IS NULL;

-- name: StopSandbox :execrows
UPDATE sandboxes s SET stopped_at = now()
FROM project_users pu
WHERE s.id = $1 AND pu.project_id = s.project_id AND pu.user_id = $2
  AND s.stopped_at IS NULL;

-- name: RestartSandbox :one
UPDATE sandboxes s SET stopped_at = NULL
FROM project_users pu
WHERE s.id = $1 AND s.stopped_at IS NOT NULL
  AND pu.project_id = s.project_id AND pu.user_id = $2
RETURNING s.id, s.project_id, s.user_id, s.created_at, s.stopped_at, s.name;