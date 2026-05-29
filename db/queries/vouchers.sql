-- name: CreateVoucher :one
INSERT INTO vouchers (id, user_id, challenge_id, session_id, code, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id;

-- name: ListVouchersByUser :many
SELECT id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id
FROM vouchers
WHERE user_id = $1
ORDER BY issued_at DESC;

-- name: GetVoucherForUpdate :one
SELECT id, user_id, challenge_id, session_id, code, status, issued_at, redeemed_at, redeemed_by_employee_id
FROM vouchers
WHERE code = $1
FOR UPDATE;

-- name: MarkVoucherRedeemed :exec
UPDATE vouchers
SET status = 'redeemed', redeemed_at = now(), redeemed_by_employee_id = $2
WHERE id = $1;

-- name: CreateVoucherRedemption :one
INSERT INTO voucher_redemptions (id, voucher_id, employee_id)
VALUES ($1, $2, $3)
RETURNING id, voucher_id, employee_id, redeemed_at;
