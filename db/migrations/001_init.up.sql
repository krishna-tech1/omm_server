CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE user_role AS ENUM ('user', 'merchant', 'employee', 'admin');

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    phone text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    avatar_url text NOT NULL DEFAULT '',
    role user_role NOT NULL DEFAULT 'user',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_categories (
    name text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO merchant_categories (name) VALUES
    ('Uncategorized'),
    ('Lifestyle'),
    ('Sports Equipment'),
    ('Fitness'),
    ('Outdoors'),
    ('Food & Beverage'),
    ('Apparel');

CREATE TABLE merchants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    category text NOT NULL DEFAULT 'Uncategorized' REFERENCES merchant_categories(name) ON UPDATE CASCADE ON DELETE RESTRICT,
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

CREATE INDEX session_checkpoints_session_idx ON session_checkpoints(session_id, recorded_at);

CREATE TABLE employees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    phone text NOT NULL,
    code text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, code),
    UNIQUE (user_id)
);

CREATE INDEX employees_merchant_idx ON employees(merchant_id);
CREATE INDEX employees_user_idx ON employees(user_id);
CREATE INDEX employees_phone_idx ON employees(phone);

CREATE TABLE coupons (
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

CREATE TABLE coupon_redemptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id uuid NOT NULL REFERENCES coupons(id),
    employee_id uuid NOT NULL REFERENCES employees(id),
    redeemed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (coupon_id)
);

CREATE INDEX coupons_user_idx ON coupons(user_id);
CREATE INDEX coupons_challenge_idx ON coupons(challenge_id);
CREATE UNIQUE INDEX coupons_user_challenge_unique_idx ON coupons(user_id, challenge_id);
