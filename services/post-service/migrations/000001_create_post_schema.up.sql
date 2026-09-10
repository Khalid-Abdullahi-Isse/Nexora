CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_user_id UUID NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT posts_content_not_blank CHECK (BTRIM(content) <> '')
);

CREATE INDEX posts_author_created_at_idx ON posts (author_user_id, created_at DESC);
CREATE INDEX posts_created_at_idx ON posts (created_at DESC);

CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_user_id UUID NOT NULL,
    parent_comment_id UUID REFERENCES comments(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT comments_content_not_blank CHECK (BTRIM(content) <> ''),
    CONSTRAINT comments_not_own_parent CHECK (parent_comment_id IS NULL OR parent_comment_id <> id)
);

CREATE INDEX comments_post_created_at_idx ON comments (post_id, created_at ASC);
CREATE INDEX comments_author_created_at_idx ON comments (author_user_id, created_at DESC);
CREATE INDEX comments_parent_comment_id_idx ON comments (parent_comment_id) WHERE parent_comment_id IS NOT NULL;

CREATE TABLE likes (
    user_id UUID NOT NULL,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);

CREATE INDEX likes_post_created_at_idx ON likes (post_id, created_at DESC);
