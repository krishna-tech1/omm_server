-- name: AdminStats :one
SELECT
    (SELECT COUNT(*) FROM users) AS total_users,
    (SELECT COUNT(*) FROM merchants) AS total_merchants,
    (SELECT COALESCE(SUM(distance_miles), 0)::double precision FROM sessions
     WHERE status IN ('completed', 'suspicious', 'void')) AS total_distance_miles,
    (SELECT COUNT(*) FROM users WHERE is_premium = true) AS premium_users;

-- name: ListAllMerchantsWithAnalytics :many
SELECT m.*,
  (SELECT COUNT(*) FROM coupon_redemptions cr
   JOIN coupons cpn ON cpn.id = cr.coupon_id
   JOIN challenges c ON c.id = cpn.challenge_id
   WHERE c.merchant_id = m.id) AS total_redemptions,
  (SELECT COUNT(*) FROM coupon_redemptions cr
   JOIN coupons cpn ON cpn.id = cr.coupon_id
   JOIN challenges c ON c.id = cpn.challenge_id
   WHERE c.merchant_id = m.id
     AND cr.redeemed_at >= NOW() - INTERVAL '7 days') AS weekly_redemptions
FROM merchants m
ORDER BY m.created_at DESC;

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
