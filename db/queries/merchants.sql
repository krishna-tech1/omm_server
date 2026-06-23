-- name: CreateMerchant :one
INSERT INTO merchants (id, owner_user_id, name, category, address_lat, address_lng, logo_url, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, owner_user_id, name, category, address_lat, address_lng, logo_url, description, created_at;

-- name: GetMerchantByOwner :one
SELECT id, owner_user_id, name, category, address_lat, address_lng, logo_url, description, created_at FROM merchants
WHERE owner_user_id = $1;

-- name: GetMerchantByID :one
SELECT id, owner_user_id, name, category, address_lat, address_lng, logo_url, description, created_at FROM merchants
WHERE id = $1;

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
     WHERE c.merchant_id = $1) AS active_customers,
    (SELECT
       CASE WHEN COUNT(DISTINCT cr2.user_id) = 0 THEN 0.0
       ELSE (SELECT COUNT(DISTINCT cpn.user_id) FROM coupons cpn
             JOIN challenges c2 ON c2.id = cpn.challenge_id
             WHERE c2.merchant_id = $1 AND cpn.status = 'redeemed')::double precision
            / COUNT(DISTINCT cr2.user_id)::double precision
       END
     FROM challenge_registrations cr2
     JOIN challenges c3 ON c3.id = cr2.challenge_id
     WHERE c3.merchant_id = $1) AS conversion_rate;

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

-- name: UpdateMerchantProfile :one
UPDATE merchants
SET name = $2, category = $3, address_lat = $4, address_lng = $5, logo_url = $6, description = $7
WHERE id = $1
RETURNING *;

-- name: GetNearbyMerchants :many
SELECT id, owner_user_id, name, category, address_lat, address_lng, logo_url, description, created_at,
    (3959 * acos(cos(radians($1::double precision)) * cos(radians(address_lat)) * cos(radians(address_lng) - radians($2::double precision)) + sin(radians($1::double precision)) * sin(radians(address_lat))))::double precision AS distance_miles
FROM merchants
WHERE (3959 * acos(cos(radians($1::double precision)) * cos(radians(address_lat)) * cos(radians(address_lng) - radians($2::double precision)) + sin(radians($1::double precision)) * sin(radians(address_lat)))) <= $3::double precision
ORDER BY distance_miles ASC;
