#!/usr/bin/env python3
"""Run race/integration tests against disposable PostgreSQL and Redis containers.
Never reads .env or connects to the development/production database.
"""
import os
from pathlib import Path
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
containers = []
def run(args, **kw):
    return subprocess.run(args, cwd=ROOT, check=True, **kw)
def start(image, port, *args):
    name = 'notification-test-' + uuid.uuid4().hex[:12]
    run(['docker', 'run', '--rm', '-d', '--name', name,
         '-p', f'127.0.0.1::{port}', *args, image], stdout=subprocess.DEVNULL)
    containers.append(name)
    published = subprocess.check_output(['docker', 'port', name, str(port)], text=True).strip()
    return name, published.rsplit(':', 1)[1]
try:
    password = uuid.uuid4().hex
    pg, pgport = start('postgres:17-alpine', 5432, '-e', 'POSTGRES_PASSWORD=' + password, '-e', 'POSTGRES_DB=notification_test')
    redis, redisport = start('redis:8-alpine', 6379)
    for _ in range(60):
        if subprocess.run(['docker', 'exec', pg, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            break
        time.sleep(0.5)
    else:
        raise RuntimeError('PostgreSQL startup timed out')
    env = dict(os.environ, APP_ENV='test', ENV_FILE='/dev/null', DATABASE_URL='', POSTGRES_HOST='127.0.0.1', POSTGRES_PORT=pgport,
               POSTGRES_USER='postgres', POSTGRES_PASSWORD=password,
               POSTGRES_DB='notification_test', POSTGRES_SSLMODE='disable',
               RESOURCE_TEST_DATABASE_URL=f'postgres://postgres:{password}@127.0.0.1:{pgport}/notification_test?sslmode=disable',
               NOTIFICATION_TEST_REDIS_ADDR=f'127.0.0.1:{redisport}')
    # Runtime roles exercise additive migration grants as well as owner tests.
    run(['docker', 'exec', '-i', pg, 'psql', '-U', 'postgres', '-d', 'notification_test', '-v', 'ON_ERROR_STOP=1'],
        input='CREATE ROLE app_notification; CREATE ROLE app_post; CREATE ROLE app_chat;\n', text=True, stdout=subprocess.DEVNULL)
    run(['go', 'run', './shared/cmd/migrate', 'all', 'up'], env=env)
    run(['go', 'test', '-race', '-count=1', './services/notification-service/...',
         './services/post-service/...', './services/chat-service/...', './shared/...'], env=env)
finally:
    for name in reversed(containers):
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
