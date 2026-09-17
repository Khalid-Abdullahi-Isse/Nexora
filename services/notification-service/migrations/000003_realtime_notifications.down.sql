BEGIN;
-- Intentionally fails rather than deleting new notification types during rollback.
ALTER TABLE notifications DROP CONSTRAINT notifications_type_valid;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_valid CHECK(type IN ('follow','like','comment','message'));
DROP TABLE notification_processed_events;
DROP INDEX notifications_user_cursor_idx;
ALTER TABLE notifications DROP COLUMN actor_id, DROP COLUMN entity_id, DROP COLUMN entity_type, DROP COLUMN event_id;
COMMIT;
