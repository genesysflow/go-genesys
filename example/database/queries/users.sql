-- User queries for example app (SQLite)

-- name: GetUser :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY name;

-- name: CreateUser :one
INSERT INTO users (name, email, birthdate, created_at, updated_at)
VALUES (?, ?, ?, datetime('now'), datetime('now'))
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name = ?, email = ?, birthdate = ?, updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UserExistsByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = ?);

-- name: UserExistsByEmailExcluding :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = ? AND id != ?);
