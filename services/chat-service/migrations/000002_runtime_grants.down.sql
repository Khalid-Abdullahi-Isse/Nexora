BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_chat') THEN
  REVOKE ALL ON conversations, conversation_members, messages FROM app_chat;
 END IF;
END $$;
COMMIT;
