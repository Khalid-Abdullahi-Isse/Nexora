-- Image URLs cannot be recovered after rollback; use only after a reviewed backup.
BEGIN;
SET LOCAL lock_timeout = '5s';
DROP INDEX IF EXISTS posts_author_feed_order_idx;
DROP INDEX IF EXISTS posts_feed_order_idx;
ALTER TABLE posts DROP COLUMN IF EXISTS image_url;
COMMIT;
