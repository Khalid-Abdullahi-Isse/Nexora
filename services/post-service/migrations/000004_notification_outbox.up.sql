BEGIN;
CREATE TABLE post_notification_outbox (
 event_id UUID PRIMARY KEY, payload JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX post_notification_outbox_created_idx ON post_notification_outbox(created_at,event_id);
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_post') THEN
 GRANT SELECT,INSERT,DELETE ON post_notification_outbox TO app_post;
 END IF;
END $$;
COMMIT;
