CREATE TABLE profiles (
    user_id UUID PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    display_name VARCHAR(150) NOT NULL,
    bio VARCHAR(500),
    avatar_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT profiles_username_format CHECK (username ~ '^[A-Za-z0-9_]{3,50}$'),
    CONSTRAINT profiles_display_name_not_blank CHECK (BTRIM(display_name) <> '')
);

CREATE UNIQUE INDEX profiles_username_unique_idx ON profiles (LOWER(username));

CREATE TABLE follows (
    follower_user_id UUID NOT NULL,
    followed_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_user_id, followed_user_id),
    CONSTRAINT follows_not_self CHECK (follower_user_id <> followed_user_id)
);

CREATE INDEX follows_followed_user_id_created_at_idx
    ON follows (followed_user_id, created_at DESC);
