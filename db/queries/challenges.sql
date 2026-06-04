-- name: ListActiveChallenges :many
SELECT id, merchant_id, title, description, target_miles, expires_at, created_at
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
SELECT id, merchant_id, title, description, target_miles, expires_at, created_at
FROM challenges
WHERE id = $1;

-- name: CreateChallenge :one
INSERT INTO challenges (id, merchant_id, title, description, target_miles, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, merchant_id, title, description, target_miles, expires_at, created_at;

-- name: RegisterChallenge :one
INSERT INTO challenge_registrations (id, challenge_id, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (challenge_id, user_id) DO UPDATE SET registered_at = challenge_registrations.registered_at
RETURNING id, challenge_id, user_id, registered_at, status;

-- name: MarkChallengeRegistrationCompleted :exec
UPDATE challenge_registrations
SET status = 'completed'
WHERE user_id = $1 AND challenge_id = $2;

-- name: MarkChallengeRegistrationCompletedIfActive :exec
UPDATE challenge_registrations
SET status = 'completed'
WHERE user_id = $1 AND challenge_id = $2 AND status <> 'completed';

-- name: ListMerchantChallengesWithCounts :many
SELECT c.id, c.merchant_id, c.title, c.description, c.target_miles, c.expires_at, c.created_at,
       COALESCE(r.participants, 0) AS participants
FROM challenges c
LEFT JOIN (
    SELECT challenge_id, COUNT(*) AS participants
    FROM challenge_registrations
    GROUP BY challenge_id
) r ON r.challenge_id = c.id
WHERE c.merchant_id = $1
ORDER BY c.created_at DESC;
