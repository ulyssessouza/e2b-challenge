-- name: CreateProject :one
INSERT INTO projects (name) VALUES ($1) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1 LIMIT 1;

-- name: ListProjectsByUser :many
SELECT p.* FROM projects p
JOIN project_users pu ON pu.project_id = p.id
WHERE pu.user_id = $1
ORDER BY p.created_at DESC, p.id DESC
LIMIT $2 OFFSET $3;

-- name: CountProjectsByUser :one
SELECT COUNT(*) FROM projects p
JOIN project_users pu ON pu.project_id = p.id
WHERE pu.user_id = $1;

-- name: AddProjectMember :exec
INSERT INTO project_users (project_id, user_id, role) VALUES ($1, $2, $3);

-- name: GetProjectMembership :one
SELECT pu.role FROM projects p
LEFT JOIN project_users pu ON pu.project_id = p.id AND pu.user_id = $2
WHERE p.id = $1;