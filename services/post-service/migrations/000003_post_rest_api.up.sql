BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE posts ADD COLUMN IF NOT EXISTS image_url TEXT;
CREATE INDEX IF NOT EXISTS posts_feed_order_idx ON posts (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS posts_author_feed_order_idx ON posts (author_user_id, created_at DESC, id DESC);
COMMIT;
