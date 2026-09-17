BEGIN;
ALTER TABLE notifications DROP CONSTRAINT notifications_type_valid;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_valid CHECK (type IN
 ('follow','like','comment','message','post_like','post_comment','post_mention','chat_message','user_follow','follow_request','system_announcement'));
ALTER TABLE notifications ADD COLUMN actor_id UUID, ADD COLUMN entity_id UUID,
 ADD COLUMN entity_type VARCHAR(40), ADD COLUMN event_id UUID;
CREATE UNIQUE INDEX notifications_event_id_idx ON notifications(event_id) WHERE event_id IS NOT NULL;
CREATE INDEX notifications_user_cursor_idx ON notifications(user_id, created_at DESC, id DESC);
-- Independent receipt survives notification deletion, preventing replay resurrection.
CREATE TABLE notification_processed_events (
 event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_notification') THEN
 GRANT SELECT,INSERT ON notification_processed_events TO app_notification;
 END IF;
END $$;
COMMIT;
