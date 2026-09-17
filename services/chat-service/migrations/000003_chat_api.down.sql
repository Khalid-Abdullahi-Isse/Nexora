BEGIN;
DROP TABLE message_receipts;
DROP INDEX messages_history_idx;
DROP INDEX messages_created_at_idx;
ALTER TABLE messages DROP CONSTRAINT chat_reply_fk, DROP CONSTRAINT chat_message_conversation_unique, DROP CONSTRAINT chat_sender_user_fk,
 DROP COLUMN message_type, DROP COLUMN reply_to_message_id, DROP COLUMN deleted_at;
ALTER TABLE conversation_members DROP CONSTRAINT chat_member_user_fk, DROP COLUMN id;
ALTER TABLE conversations DROP CONSTRAINT chat_creator_user_fk, DROP COLUMN direct_pair;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='app_chat') THEN REVOKE SELECT(id,status) ON users FROM app_chat; END IF; END $$;
COMMIT;
