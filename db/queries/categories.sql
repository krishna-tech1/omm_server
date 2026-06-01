-- name: CreateCategory :one
INSERT INTO merchant_categories (name)
VALUES ($1)
RETURNING name, created_at;

-- name: ListCategories :many
SELECT name, created_at
FROM merchant_categories
ORDER BY name;

-- name: GetCategoryByName :one
SELECT name, created_at
FROM merchant_categories
WHERE name = $1;

-- name: UpdateCategory :one
UPDATE merchant_categories
SET name = $2
WHERE name = $1
RETURNING name, created_at;

-- name: DeleteCategory :one
DELETE FROM merchant_categories
WHERE name = $1
RETURNING name, created_at;
