BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_auth') THEN
  GRANT USAGE ON SCHEMA public TO app_auth;
  GRANT SELECT,INSERT,UPDATE,DELETE ON users, roles, permissions, user_roles, role_permissions, sessions, session_families TO app_auth;
  GRANT INSERT ON security_audit TO app_auth;
 END IF;
END $$;
COMMIT;
