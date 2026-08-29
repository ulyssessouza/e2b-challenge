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
UPDATE sandboxes s SET stopped_at = now(), version = version + 1
FROM project_users pu
WHERE s.id = $1 AND pu.project_id = s.project_id AND pu.user_id = $2
  AND s.stopped_at IS NULL;

-- name: RestartSandbox :one
UPDATE sandboxes s SET stopped_at = NULL, version = version + 1
FROM project_users pu
WHERE s.id = $1 AND s.version = $2
  AND pu.project_id = s.project_id AND pu.user_id = $3
RETURNING s.id, s.project_id, s.user_id, s.created_at, s.stopped_at, s.version, s.name;