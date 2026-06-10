ALTER TABLE users
    ADD COLUMN is_banned boolean NOT NULL DEFAULT false,
    ADD COLUMN is_premium boolean NOT NULL DEFAULT false;

ALTER TABLE challenges
    ADD COLUMN duration_days integer NOT NULL DEFAULT 0,
    ADD COLUMN reward text NOT NULL DEFAULT '',
    ADD COLUMN reward_image_url text NOT NULL DEFAULT '';
