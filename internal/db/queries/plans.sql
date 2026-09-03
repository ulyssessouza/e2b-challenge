-- name: GetUserPlan :one
SELECT p.id, p.name, p.max_projects, p.max_running_sandboxes, p.created_at
FROM users u
JOIN plans p ON p.id = u.plan_id
WHERE u.id = $1;