-- name: CreateSession :one
INSERT INTO sessions (id, user_id, challenge_id, start_time, start_lat, start_lng, steps_start, miles_start, status, public_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, user_id, challenge_id, start_time, end_time, start_lat, start_lng, end_lat, end_lng,
          steps_start, steps_end, miles_start, miles_end, distance_miles, status, public_key, created_at, updated_at;

-- name: GetSessionByID :one
SELECT * FROM sessions
WHERE id = $1;

-- name: GetSessionForUser :one
SELECT * FROM sessions
WHERE id = $1 AND user_id = $2;

-- name: GetActiveSessionStreamMeta :one
SELECT s.id, s.user_id, s.challenge_id, s.start_time, s.start_lat, s.start_lng,
       s.steps_start, s.miles_start, s.distance_miles, s.status, s.public_key,
       c.title AS challenge_title, c.target_miles, c.expires_at,
       cr.status AS registration_status, cr.distance_covered
FROM sessions s
JOIN challenges c ON c.id = s.challenge_id
JOIN challenge_registrations cr ON cr.challenge_id = s.challenge_id AND cr.user_id = s.user_id
WHERE s.id = $1
  AND s.user_id = $2
  AND s.status = 'active'
  AND c.expires_at > now();

-- name: UpdateSessionEnd :exec
UPDATE sessions
SET end_time = $2, end_lat = $3, end_lng = $4, steps_end = $5, miles_end = $6, updated_at = now()
WHERE id = $1;

-- name: UpdateSessionStatusAndDistance :exec
UPDATE sessions
SET status = $2, distance_miles = $3, updated_at = now()
WHERE id = $1;

-- name: UpdateSessionProgress :exec
UPDATE sessions
SET distance_miles = GREATEST(distance_miles, $2), updated_at = now()
WHERE id = $1;

-- name: CreateSessionCheckpoint :one
INSERT INTO session_checkpoints (session_id, lat, lng, recorded_at, steps, distance_meters, speed_mps, speed_violation)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (session_id, recorded_at) DO UPDATE
SET lat = EXCLUDED.lat,
    lng = EXCLUDED.lng,
    steps = EXCLUDED.steps,
    distance_meters = EXCLUDED.distance_meters,
    speed_mps = EXCLUDED.speed_mps,
    speed_violation = EXCLUDED.speed_violation
RETURNING id, session_id, lat, lng, recorded_at, steps, distance_meters, speed_mps, speed_violation, created_at;

-- name: GetUserTotalDuration :one
SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (end_time - start_time))), 0)::bigint
  AS total_duration_seconds
FROM sessions
WHERE user_id = $1 AND status IN ('completed', 'suspicious', 'void')
  AND end_time IS NOT NULL;

-- name: GetUserWeeklyMiles :many
SELECT
  TO_CHAR(s.start_time AT TIME ZONE 'UTC', 'DY') AS day_name,
  COALESCE(SUM(s.distance_miles), 0)::double precision AS miles
FROM sessions s
WHERE s.user_id = $1
  AND s.status IN ('completed', 'suspicious', 'void')
  AND s.start_time >= NOW() - INTERVAL '7 days'
GROUP BY day_name, DATE_TRUNC('day', s.start_time AT TIME ZONE 'UTC')
ORDER BY DATE_TRUNC('day', s.start_time AT TIME ZONE 'UTC');

-- name: FindStaleSessions :many
SELECT s.* FROM sessions s
LEFT JOIN LATERAL (
  SELECT MAX(created_at) AS last_checkpoint_at
  FROM session_checkpoints WHERE session_id = s.id
) sc ON true
WHERE s.status = 'active'
  AND COALESCE(sc.last_checkpoint_at, s.created_at) < NOW() - ($1::int * INTERVAL '1 minute');
