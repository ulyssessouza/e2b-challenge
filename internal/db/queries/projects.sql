-- name: CreateProject :one
INSERT INTO projects (owner_id, name) VALUES ($1, $2) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1 LIMIT 1;

-- name: GetProjectMembership :one
-- One round-trip answers both authorization questions: does the project
-- exist, and what is the caller's derived role (ownership is an attribute
-- of the project; project_users is the access list). member_user_id is
-- NULL when the caller is not a member.
SELECT pu.user_id AS member_user_id,
       (p.owner_id = sqlc.arg(caller_id))::bool AS is_owner
FROM projects p
LEFT JOIN project_users pu ON pu.project_id = p.id AND pu.user_id = sqlc.arg(caller_id)
WHERE p.id = sqlc.arg(project_id);

-- name: CountProjectsByOwner :one
-- The LIMIT caps the scan at the plan cap (see CountRunningSandboxesByUser).
SELECT COUNT(*) FROM (
    SELECT 1 FROM projects
    WHERE owner_id = $1
    LIMIT $2
) owned;

-- name: CountProjectsByUser :one
SELECT COUNT(*) FROM projects p
JOIN project_users pu ON pu.project_id = p.id
WHERE pu.user_id = $1;

-- name: ListProjectsByUser :many
SELECT p.* FROM projects p
JOIN project_users pu ON pu.project_id = p.id
WHERE pu.user_id = $1
ORDER BY p.created_at DESC, p.id DESC
LIMIT $2 OFFSET $3;

-- name: CreateProjectMember :exec
INSERT INTO project_users (project_id, user_id) VALUES ($1, $2);