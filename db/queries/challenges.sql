-- name: ListActiveChallenges :many
SELECT *
FROM challenges
WHERE expires_at > $1
ORDER BY created_at DESC;

-- name: ListActiveChallengeIDsForUser :many
SELECT cr.challenge_id
FROM challenge_registrations cr
JOIN challenges c ON c.id = cr.challenge_id
WHERE cr.user_id = $1
    AND cr.status IN ('active', 'pending')
    AND c.expires_at > $2;

-- name: GetChallengeByID :one
SELECT *
FROM challenges
WHERE id = $1;

-- name: CreateChallenge :one
INSERT INTO challenges (id, merchant_id, title, description, target_miles, expires_at, duration_days, reward, reward_image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateChallenge :one
UPDATE challenges
SET title = $2, description = $3, target_miles = $4, expires_at = $5, duration_days = $6, reward = $7, reward_image_url = $8
WHERE id = $1 AND merchant_id = $9
RETURNING *;

-- name: CountActiveChallengeRegistrationsForUser :one
SELECT COUNT(*)
FROM challenge_registrations cr
JOIN challenges c ON c.id = cr.challenge_id
WHERE cr.user_id = $1
  AND cr.status IN ('active', 'pending')
  AND c.expires_at > $2;

-- name: RegisterChallenge :one
INSERT INTO challenge_registrations (id, challenge_id, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (challenge_id, user_id) DO UPDATE SET registered_at = challenge_registrations.registered_at
RETURNING *;

-- name: MarkChallengeRegistrationCompleted :exec
UPDATE challenge_registrations
SET status = 'completed'
WHERE user_id = $1 AND challenge_id = $2;

-- name: MarkChallengeRegistrationCompletedIfActive :exec
UPDATE challenge_registrations
SET status = 'completed'
WHERE user_id = $1 AND challenge_id = $2 AND status <> 'completed';

-- name: ListMerchantChallengesWithStats :many
SELECT c.*,
  (SELECT COUNT(*) FROM challenge_registrations cr
   WHERE cr.challenge_id = c.id) AS participants,
  (SELECT CASE WHEN COUNT(*) = 0 THEN 0.0
   ELSE COUNT(*) FILTER (WHERE cr.status = 'completed')::double precision
        / COUNT(*)::double precision END
   FROM challenge_registrations cr
   WHERE cr.challenge_id = c.id) AS completion_rate,
  (SELECT COUNT(*) FROM coupon_redemptions crd
   JOIN coupons cpn ON cpn.id = crd.coupon_id
   WHERE cpn.challenge_id = c.id) AS redeemed_count
FROM challenges c
WHERE c.merchant_id = $1
ORDER BY c.created_at DESC;

-- name: GetChallengeRegistrationForUser :one
SELECT * FROM challenge_registrations
WHERE challenge_id = $1 AND user_id = $2;

-- name: GetChallengeRegistrationsForUser :many
SELECT * FROM challenge_registrations
WHERE user_id = $1;

-- name: AddRegistrationDistance :exec
UPDATE challenge_registrations
SET distance_covered = distance_covered + $3
WHERE challenge_id = $1 AND user_id = $2;

-- name: ExpireOverdueRegistrations :exec
UPDATE challenge_registrations cr
SET status = 'expired'
FROM challenges c
WHERE cr.challenge_id = c.id
  AND cr.status IN ('active', 'pending')
  AND cr.registered_at + (c.duration_days * INTERVAL '1 day') < NOW();
