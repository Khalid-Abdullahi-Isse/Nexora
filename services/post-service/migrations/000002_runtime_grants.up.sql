BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_post') THEN
  GRANT USAGE ON SCHEMA public TO app_post;
  GRANT SELECT,INSERT,UPDATE,DELETE ON posts, comments, likes TO app_post;
  
 END IF;
END $$;
COMMIT;
