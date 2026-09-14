BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_chat') THEN
  GRANT USAGE ON SCHEMA public TO app_chat;
  GRANT SELECT,INSERT,UPDATE,DELETE ON conversations, conversation_members, messages TO app_chat;
  
 END IF;
END $$;
COMMIT;
