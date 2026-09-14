BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_notification') THEN
  GRANT USAGE ON SCHEMA public TO app_notification;
  GRANT SELECT,INSERT,UPDATE,DELETE ON notifications, notification_preferences, notification_deliveries TO app_notification;
  
 END IF;
END $$;
COMMIT;
