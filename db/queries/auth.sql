-- name: CreateOTP :one
INSERT INTO otps (id, phone, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, phone, code_hash, expires_at, used_at, created_at;

-- name: GetOTPForVerify :one
SELECT id, phone, code_hash, expires_at, used_at
FROM otps
WHERE id = $1 AND phone = $2
LIMIT 1;

-- name: MarkOTPUsed :exec
UPDATE otps
SET used_at = now()
WHERE id = $1;
