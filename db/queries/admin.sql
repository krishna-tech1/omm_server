-- name: AdminStats :one
SELECT
    (SELECT COUNT(*) FROM users) AS total_users,
    (SELECT COUNT(*) FROM merchants) AS total_merchants,
    (SELECT COALESCE(SUM(distance_miles), 0)::double precision FROM sessions
     WHERE status IN ('completed', 'suspicious', 'void')) AS total_distance_miles;

-- name: ListAllMerchants :many
SELECT * FROM merchants ORDER BY created_at DESC;

-- name: BanUser :exec
UPDATE users SET is_banned = true WHERE id = $1;

-- name: CancelMerchantChallenges :exec
UPDATE challenges
SET expires_at = NOW()
WHERE merchant_id = $1;

-- name: GetAffectedUsersByMerchantBan :many
SELECT DISTINCT u.phone
FROM challenge_registrations cr
JOIN challenges c ON c.id = cr.challenge_id
JOIN users u ON u.id = cr.user_id
WHERE c.merchant_id = $1 AND cr.status IN ('active', 'pending');
