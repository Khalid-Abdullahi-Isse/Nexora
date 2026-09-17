-- Refuse to discard unpublished events.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM post_notification_outbox) THEN RAISE EXCEPTION 'Drain outbox before rollback'; END IF;
END $$;
DROP TABLE post_notification_outbox;
