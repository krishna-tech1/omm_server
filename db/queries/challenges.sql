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

-- name: ListMerchantChallengesWithCounts :many
SELECT c.id, c.merchant_id, c.title, c.description, c.target_miles, c.expires_at, c.duration_days, c.reward, c.reward_image_url, c.created_at,
       COALESCE(r.participants, 0) AS participants
FROM challenges c
LEFT JOIN (
    SELECT challenge_id, COUNT(*) AS participants
    FROM challenge_registrations
    GROUP BY challenge_id
) r ON r.challenge_id = c.id
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
