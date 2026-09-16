BEGIN;
ALTER TABLE conversations ADD COLUMN direct_pair TEXT;
-- Preserve legacy duplicates; adopt the oldest active two-member direct chat.
WITH pairs AS (
 SELECT c.id, string_agg(m.user_id::text, ':' ORDER BY m.user_id::text) AS pair,
 row_number() OVER (PARTITION BY string_agg(m.user_id::text, ':' ORDER BY m.user_id::text) ORDER BY c.created_at,c.id) AS rank
 FROM conversations c JOIN conversation_members m ON m.conversation_id=c.id AND m.left_at IS NULL
 WHERE c.type='direct' GROUP BY c.id HAVING count(*)=2
) UPDATE conversations c SET direct_pair=p.pair FROM pairs p WHERE c.id=p.id AND p.rank=1;
CREATE UNIQUE INDEX conversations_direct_pair_unique ON conversations(direct_pair) WHERE direct_pair IS NOT NULL;
ALTER TABLE conversation_members ADD COLUMN id UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE;
-- NOT VALID preserves historical orphan records while enforcing new writes.
ALTER TABLE conversation_members ADD CONSTRAINT chat_member_user_fk FOREIGN KEY(user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE conversations ADD CONSTRAINT chat_creator_user_fk FOREIGN KEY(created_by_user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE messages ADD COLUMN message_type VARCHAR(20) NOT NULL DEFAULT 'text' CHECK(message_type IN ('text','system')),
 ADD COLUMN reply_to_message_id UUID, ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE messages ADD CONSTRAINT chat_sender_user_fk FOREIGN KEY(sender_user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE messages ADD CONSTRAINT chat_message_conversation_unique UNIQUE(id,conversation_id);
ALTER TABLE messages ADD CONSTRAINT chat_reply_fk FOREIGN KEY(reply_to_message_id,conversation_id) REFERENCES messages(id,conversation_id);
CREATE INDEX messages_history_idx ON messages(conversation_id,created_at DESC,id DESC) WHERE deleted_at IS NULL;
CREATE INDEX messages_created_at_idx ON messages(created_at);
CREATE TABLE message_receipts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 message_id UUID NOT NULL REFERENCES messages(id),
 user_id UUID NOT NULL REFERENCES users(id),
 delivered_at TIMESTAMPTZ, read_at TIMESTAMPTZ,
 UNIQUE(message_id,user_id),
 CHECK(read_at IS NULL OR (delivered_at IS NOT NULL AND read_at>=delivered_at))
);
CREATE INDEX message_receipts_user_idx ON message_receipts(user_id,message_id);
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='app_chat') THEN
 GRANT SELECT,INSERT,UPDATE ON message_receipts TO app_chat;
 GRANT SELECT(id,status) ON users TO app_chat;
END IF; END $$;
COMMIT;
