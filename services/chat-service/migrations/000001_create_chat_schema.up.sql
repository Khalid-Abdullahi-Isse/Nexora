CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(20) NOT NULL,
    title VARCHAR(150),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT conversations_type_valid CHECK (type IN ('direct', 'group')),
    CONSTRAINT conversations_title_valid CHECK (
        (type = 'direct' AND title IS NULL) OR
        (type = 'group' AND title IS NOT NULL AND BTRIM(title) <> '')
    )
);

CREATE INDEX conversations_creator_created_at_idx
    ON conversations (created_by_user_id, created_at DESC);

CREATE TABLE conversation_members (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    PRIMARY KEY (conversation_id, user_id),
    CONSTRAINT conversation_members_role_valid CHECK (role IN ('member', 'admin')),
    CONSTRAINT conversation_members_left_at_valid CHECK (left_at IS NULL OR left_at >= joined_at)
);

CREATE INDEX conversation_members_user_id_idx
    ON conversation_members (user_id, conversation_id) WHERE left_at IS NULL;

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
    sender_user_id UUID NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT messages_content_not_blank CHECK (BTRIM(content) <> '')
);

CREATE INDEX messages_conversation_created_at_idx
    ON messages (conversation_id, created_at ASC);
CREATE INDEX messages_sender_created_at_idx
    ON messages (sender_user_id, created_at DESC);
