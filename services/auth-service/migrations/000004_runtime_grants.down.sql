BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='app_auth') THEN
  REVOKE ALL ON users, roles, permissions, user_roles, role_permissions, sessions, session_families, security_audit FROM app_auth;
 END IF;
END $$;
COMMIT;
