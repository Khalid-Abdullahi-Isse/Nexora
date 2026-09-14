BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_post') THEN
  REVOKE ALL ON posts, comments, likes FROM app_post;
 END IF;
END $$;
COMMIT;
