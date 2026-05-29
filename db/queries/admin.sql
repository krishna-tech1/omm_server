-- name: AdminStats :one
SELECT
    (SELECT COUNT(*) FROM users) AS total_users,
    (SELECT COUNT(*) FROM merchants) AS total_merchants,
    (SELECT COALESCE(SUM(distance_miles), 0)::double precision FROM sessions
     WHERE status IN ('completed', 'suspicious', 'void')) AS total_distance_miles;
