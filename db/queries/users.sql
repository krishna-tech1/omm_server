-- name: GetUserByPhone :one
SELECT *
FROM users
WHERE phone = $1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (id, phone, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET name = $2, avatar_url = $3
WHERE id = $1
RETURNING *;

-- name: UpdateUserRole :exec
UPDATE users
SET role = $2
WHERE id = $1;

-- name: UpgradeUserToPremium :exec
UPDATE users
SET is_premium = true
WHERE id = $1;
