CREATE TABLE IF NOT EXISTS session_checkpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    recorded_at timestamptz NOT NULL,
    steps integer NOT NULL DEFAULT 0,
    distance_meters double precision NOT NULL DEFAULT 0,
    speed_mps double precision NOT NULL DEFAULT 0,
    speed_violation boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, recorded_at)
);

CREATE INDEX IF NOT EXISTS session_checkpoints_session_idx ON session_checkpoints(session_id, recorded_at);
CREATE UNIQUE INDEX IF NOT EXISTS coupons_user_challenge_unique_idx ON coupons(user_id, challenge_id);