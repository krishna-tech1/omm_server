ALTER TABLE challenges
    DROP COLUMN reward_image_url,
    DROP COLUMN reward,
    DROP COLUMN duration_days;

ALTER TABLE users
    DROP COLUMN is_premium,
    DROP COLUMN is_banned;
