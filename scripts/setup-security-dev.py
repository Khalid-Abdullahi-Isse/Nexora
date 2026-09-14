#!/usr/bin/env python3
"""Generate local-only credentials; never overwrite an existing environment."""
from pathlib import Path
import os, secrets, subprocess
root=Path(__file__).resolve().parents[1]
env=root/'.env'
if env.exists():
    raise SystemExit('.env already exists; preserve it and configure the documented security variables manually')
keys=root/'.secrets'
subprocess.run(['go','run','./shared/cmd/security-keys',str(keys)],cwd=root,check=True)
values={
 'APP_ENV':'development','POSTGRES_HOST':'localhost','POSTGRES_PORT':'15432',
 'POSTGRES_USER':'app_auth','POSTGRES_PASSWORD':secrets.token_urlsafe(32),
 'POSTGRES_ADMIN_PASSWORD':secrets.token_urlsafe(32),'POSTGRES_DB':'social_media','POSTGRES_SSLMODE':'disable',
 'POST_DATABASE_PASSWORD':secrets.token_urlsafe(32),'CHAT_DATABASE_PASSWORD':secrets.token_urlsafe(32),'NOTIFICATION_DATABASE_PASSWORD':secrets.token_urlsafe(32),
 'REDIS_PASSWORD':secrets.token_urlsafe(32),'JWT_PRIVATE_KEY_FILE':str(keys/'private.pem'),'JWT_PUBLIC_KEYS_FILE':str(keys/'public-keys.json'),
 'JWT_KEY_ID':'development-1','JWT_ISSUER':'social-media-auth','ALLOWED_ORIGINS':'http://localhost:3000','AUTH_COOKIE_INSECURE':'true',
 'LOCAL_UID':str(os.getuid()),'LOCAL_GID':str(os.getgid()),'REFRESH_TOKEN_TTL':'168h'}
values['REDIS_ADDR']='redis://:'+values['REDIS_PASSWORD']+'@localhost:6379/0'
with env.open('x') as f:
 os.chmod(env,0o600)
 f.write(''.join(k+'='+v+'\n' for k,v in values.items()))
print('Created .env and .secrets; values were not printed. Existing database passwords are not changed.')
