BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_notification') THEN
  REVOKE ALL ON notifications, notification_preferences, notification_deliveries FROM app_notification;
 END IF;
END $$;
COMMIT;
