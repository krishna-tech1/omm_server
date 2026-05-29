-- name: CreateSession :one
INSERT INTO sessions (id, user_id, challenge_id, start_time, start_lat, start_lng, steps_start, miles_start, status, hmac_secret)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, user_id, challenge_id, start_time, end_time, start_lat, start_lng, end_lat, end_lng,
          steps_start, steps_end, miles_start, miles_end, distance_miles, status, hmac_secret, created_at, updated_at;

-- name: GetSessionByID :one
SELECT * FROM sessions
WHERE id = $1;

-- name: GetSessionForUser :one
SELECT * FROM sessions
WHERE id = $1 AND user_id = $2;

-- name: UpdateSessionEnd :exec
UPDATE sessions
SET end_time = $2, end_lat = $3, end_lng = $4, steps_end = $5, miles_end = $6, updated_at = now()
WHERE id = $1;

-- name: UpdateSessionStatusAndDistance :exec
UPDATE sessions
SET status = $2, distance_miles = $3, updated_at = now()
WHERE id = $1;

-- name: GetLastCheckpoint :one
SELECT id, session_id, lat, lng, recorded_at, steps, distance_meters, speed_mps, speed_violation
FROM session_checkpoints
WHERE session_id = $1
ORDER BY recorded_at DESC
LIMIT 1;

-- name: CreateCheckpoint :exec
INSERT INTO session_checkpoints (session_id, lat, lng, recorded_at, steps, distance_meters, speed_mps, speed_violation)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: SumCheckpointDistanceMeters :one
SELECT COALESCE(SUM(distance_meters), 0)::double precision
FROM session_checkpoints
WHERE session_id = $1;
