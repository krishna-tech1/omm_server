-- name: CreateMerchant :one
INSERT INTO merchants (id, owner_user_id, name, category, address_lat, address_lng, logo_url, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, owner_user_id, name, category, address_lat, address_lng, logo_url, description, created_at;

-- name: GetMerchantByOwner :one
SELECT * FROM merchants
WHERE owner_user_id = $1;

-- name: MerchantDashboardStats :one
SELECT
    (SELECT COUNT(*) FROM challenges c WHERE c.merchant_id = $1) AS total_challenges,
    (SELECT COUNT(*) FROM coupon_redemptions cr
     JOIN coupons cpn ON cpn.id = cr.coupon_id
     JOIN challenges c ON c.id = cpn.challenge_id
     WHERE c.merchant_id = $1) AS total_redemptions,
    (SELECT COUNT(DISTINCT cr.user_id)
     FROM challenge_registrations cr
     JOIN challenges c ON c.id = cr.challenge_id
     WHERE c.merchant_id = $1) AS active_customers;

-- name: CreateEmployee :one
INSERT INTO employees (id, merchant_id, user_id, name, phone, code, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, merchant_id, user_id, name, phone, code, status, created_at;

-- name: ListEmployeesByMerchant :many
SELECT id, merchant_id, user_id, name, phone, code, status, created_at
FROM employees
WHERE merchant_id = $1
ORDER BY created_at DESC;

-- name: GetEmployeeByCode :one
SELECT id, merchant_id, user_id, name, phone, code, status, created_at
FROM employees
WHERE merchant_id = $1 AND code = $2;

-- name: GetEmployeeByPhone :one
SELECT id, merchant_id, user_id, name, phone, code, status, created_at
FROM employees
WHERE phone = $1;

-- name: GetEmployeeByUserID :one
SELECT id, merchant_id, user_id, name, phone, code, status, created_at
FROM employees
WHERE user_id = $1;

-- name: UpdateEmployee :one
UPDATE employees
SET name = COALESCE(NULLIF($3, ''), name),
    status = COALESCE(NULLIF($4, ''), status)
WHERE id = $1 AND merchant_id = $2
RETURNING id, merchant_id, user_id, name, phone, code, status, created_at;

-- name: DeactivateEmployee :one
UPDATE employees
SET status = 'inactive'
WHERE id = $1 AND merchant_id = $2
RETURNING id, merchant_id, user_id, name, phone, code, status, created_at;
