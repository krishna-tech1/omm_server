CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    phone text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    avatar_url text NOT NULL DEFAULT '',
    role text NOT NULL DEFAULT 'user',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE otps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    phone text NOT NULL,
    code_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX otps_phone_idx ON otps(phone);

CREATE TABLE merchants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    category text NOT NULL DEFAULT '',
    address_lat double precision NOT NULL DEFAULT 0,
    address_lng double precision NOT NULL DEFAULT 0,
    logo_url text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX merchants_owner_idx ON merchants(owner_user_id);

CREATE TABLE challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    target_miles double precision NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX challenges_merchant_idx ON challenges(merchant_id);
CREATE INDEX challenges_expires_idx ON challenges(expires_at);

CREATE TABLE challenge_registrations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_id uuid NOT NULL REFERENCES challenges(id),
    user_id uuid NOT NULL REFERENCES users(id),
    registered_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'active',
    UNIQUE (challenge_id, user_id)
);

CREATE INDEX registrations_user_idx ON challenge_registrations(user_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    challenge_id uuid NOT NULL REFERENCES challenges(id),
    start_time timestamptz NOT NULL,
    end_time timestamptz,
    start_lat double precision NOT NULL DEFAULT 0,
    start_lng double precision NOT NULL DEFAULT 0,
    end_lat double precision NOT NULL DEFAULT 0,
    end_lng double precision NOT NULL DEFAULT 0,
    steps_start integer NOT NULL DEFAULT 0,
    steps_end integer NOT NULL DEFAULT 0,
    miles_start double precision NOT NULL DEFAULT 0,
    miles_end double precision NOT NULL DEFAULT 0,
    distance_miles double precision NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'active',
    hmac_secret text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_challenge_idx ON sessions(challenge_id);

CREATE TABLE session_checkpoints (
    id bigserial PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    recorded_at timestamptz NOT NULL,
    steps integer NOT NULL DEFAULT 0,
    distance_meters double precision NOT NULL DEFAULT 0,
    speed_mps double precision NOT NULL DEFAULT 0,
    speed_violation boolean NOT NULL DEFAULT false
);

CREATE INDEX checkpoints_session_time_idx ON session_checkpoints(session_id, recorded_at);

CREATE TABLE employees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    name text NOT NULL,
    phone text NOT NULL DEFAULT '',
    code text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, code)
);

CREATE INDEX employees_merchant_idx ON employees(merchant_id);

CREATE TABLE vouchers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    challenge_id uuid NOT NULL REFERENCES challenges(id),
    session_id uuid NOT NULL REFERENCES sessions(id),
    code text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'active',
    issued_at timestamptz NOT NULL DEFAULT now(),
    redeemed_at timestamptz,
    redeemed_by_employee_id uuid REFERENCES employees(id)
);

CREATE TABLE voucher_redemptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    voucher_id uuid NOT NULL REFERENCES vouchers(id),
    employee_id uuid NOT NULL REFERENCES employees(id),
    redeemed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (voucher_id)
);

CREATE INDEX vouchers_user_idx ON vouchers(user_id);
