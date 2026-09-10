CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    type VARCHAR(30) NOT NULL,
    title VARCHAR(200) NOT NULL,
    message TEXT NOT NULL,
    data JSONB,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notifications_type_valid CHECK (type IN ('follow', 'like', 'comment', 'message')),
    CONSTRAINT notifications_title_not_blank CHECK (BTRIM(title) <> ''),
    CONSTRAINT notifications_message_not_blank CHECK (BTRIM(message) <> ''),
    CONSTRAINT notifications_data_object CHECK (data IS NULL OR jsonb_typeof(data) = 'object')
);

CREATE INDEX notifications_user_created_at_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_user_unread_idx ON notifications (user_id, created_at DESC) WHERE read_at IS NULL;

CREATE TABLE notification_preferences (
    user_id UUID NOT NULL,
    notification_type VARCHAR(30) NOT NULL,
    in_app_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, notification_type),
    CONSTRAINT notification_preferences_type_valid
        CHECK (notification_type IN ('follow', 'like', 'comment', 'message'))
);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel VARCHAR(20) NOT NULL DEFAULT 'in_app',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempted_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_deliveries_channel_valid CHECK (channel IN ('in_app')),
    CONSTRAINT notification_deliveries_status_valid CHECK (status IN ('pending', 'delivered', 'failed')),
    CONSTRAINT notification_deliveries_delivered_at_valid CHECK (
        (status = 'delivered' AND delivered_at IS NOT NULL) OR
        (status <> 'delivered' AND delivered_at IS NULL)
    )
);

CREATE INDEX notification_deliveries_notification_id_idx
    ON notification_deliveries (notification_id);
CREATE INDEX notification_deliveries_status_created_at_idx
    ON notification_deliveries (status, created_at ASC);
