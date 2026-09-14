BEGIN;
CREATE TABLE session_families (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 CONSTRAINT family_expiry CHECK(expires_at > created_at)
);
CREATE INDEX session_families_user_idx ON session_families(user_id,created_at DESC);
-- Pre-existing unused session records cannot be adopted as trusted login sessions.
UPDATE sessions SET revoked_at=COALESCE(revoked_at,NOW());
ALTER TABLE sessions ADD COLUMN family_id UUID REFERENCES session_families(id) ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN consumed_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN replaced_by_token_id UUID REFERENCES sessions(id);
CREATE UNIQUE INDEX session_one_active_generation ON sessions(family_id) WHERE revoked_at IS NULL AND consumed_at IS NULL;
CREATE INDEX sessions_family_idx ON sessions(family_id);
CREATE TABLE security_audit (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 actor_user_id UUID,
 target_user_id UUID,
 action VARCHAR(64) NOT NULL,
 resource_id UUID,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 ip VARCHAR(64) NOT NULL DEFAULT '',
 user_agent VARCHAR(256) NOT NULL DEFAULT ''
);
CREATE INDEX security_audit_time_idx ON security_audit(created_at);
INSERT INTO roles(id,name) VALUES ('00000000-0000-4000-8000-000000000001','user'),('00000000-0000-4000-8000-000000000002','admin') ON CONFLICT(name) DO NOTHING;
INSERT INTO permissions(code) VALUES ('accounts.read-own'),('sessions.manage-own'),('posts.read-own'),('posts.create'),('posts.update-own'),('posts.delete-own'),('chats.member'),('notifications.manage-own'),('admin.users.manage'),('system.audit.read') ON CONFLICT(code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='user' AND p.code IN ('accounts.read-own','sessions.manage-own','posts.read-own','posts.create','posts.update-own','posts.delete-own','chats.member','notifications.manage-own') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='admin' AND p.code IN ('admin.users.manage','system.audit.read') ON CONFLICT DO NOTHING;
-- Existing accounts receive only baseline user capabilities, never admin.
INSERT INTO user_roles(user_id,role_id) SELECT u.id,r.id FROM users u CROSS JOIN roles r WHERE r.name='user' ON CONFLICT DO NOTHING;
COMMIT;
