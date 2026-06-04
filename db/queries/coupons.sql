-- name: CreateCoupon :one
INSERT INTO coupons (id, user_id, challenge_id, session_id, code, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id;

-- name: CreateCouponIfNotExists :one
INSERT INTO coupons (id, user_id, challenge_id, session_id, code, status)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, challenge_id) DO UPDATE
SET issued_at = coupons.issued_at
RETURNING id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id;

-- name: GetCouponByUserChallenge :one
SELECT id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id
FROM coupons
WHERE user_id = $1 AND challenge_id = $2;

-- name: ListCouponsByUser :many
SELECT id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id
FROM coupons
WHERE user_id = $1
ORDER BY issued_at DESC;

-- name: GetCouponForUpdate :one
SELECT id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id
FROM coupons
WHERE code = $1
FOR UPDATE;

-- name: MarkCouponRedeemed :exec
UPDATE coupons
SET status = 'redeemed', redeemed_at = now(), redeemed_by_employee_id = $2
WHERE id = $1;

-- name: CreateCouponRedemption :one
INSERT INTO coupon_redemptions (id, coupon_id, employee_id)
VALUES ($1, $2, $3)
RETURNING id, coupon_id, employee_id, redeemed_at;
