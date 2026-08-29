-- name: GetUserPlan :one
SELECT p.id, p.name, p.max_projects, p.max_running_sandboxes, p.created_at
FROM users u
JOIN plans p ON p.id = u.plan_id
WHERE u.id = $1;

-- name: CountProjectsOwnedByUser :one
-- The LIMIT caps the scan: the caller only needs to know whether the count
-- reached the plan cap, so the cost stays bounded however many projects a
-- user owns.
SELECT COUNT(*) FROM (
    SELECT 1 FROM project_users
    WHERE user_id = $1 AND role = 'owner'
    LIMIT $2
) owned;