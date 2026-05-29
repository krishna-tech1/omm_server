-- name: GetUserByPhone :one
SELECT id, phone, name, avatar_url, role, created_at
FROM users
WHERE phone = $1;

-- name: GetUserByID :one
SELECT id, phone, name, avatar_url, role, created_at
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (id, phone, role)
VALUES ($1, $2, $3)
RETURNING id, phone, name, avatar_url, role, created_at;

-- name: UpdateUserProfile :one
UPDATE users
SET name = $2, avatar_url = $3
WHERE id = $1
RETURNING id, phone, name, avatar_url, role, created_at;

-- name: UpdateUserRole :exec
UPDATE users
SET role = $2
WHERE id = $1;
