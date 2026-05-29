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
    (SELECT COUNT(*) FROM voucher_redemptions vr
     JOIN vouchers v ON v.id = vr.voucher_id
     JOIN challenges c ON c.id = v.challenge_id
     WHERE c.merchant_id = $1) AS total_redemptions,
    (SELECT COUNT(DISTINCT cr.user_id)
     FROM challenge_registrations cr
     JOIN challenges c ON c.id = cr.challenge_id
     WHERE c.merchant_id = $1) AS active_customers;

-- name: CreateEmployee :one
INSERT INTO employees (id, merchant_id, name, phone, code, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, merchant_id, name, phone, code, status, created_at;

-- name: ListEmployeesByMerchant :many
SELECT id, merchant_id, name, phone, code, status, created_at
FROM employees
WHERE merchant_id = $1
ORDER BY created_at DESC;

-- name: GetEmployeeByCode :one
SELECT id, merchant_id, name, phone, code, status, created_at
FROM employees
WHERE merchant_id = $1 AND code = $2;
