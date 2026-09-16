#!/usr/bin/env python3
"""Development smoke test through Kong. Leaves one unique test account, no posts."""
import argparse
import http.cookiejar
import json
import secrets
import urllib.error
import urllib.request
import uuid

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--base-url', default='http://localhost:8000')
a = p.parse_args()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

def call(method, path, expected, body=None, headers=None):
    h = {'Origin': 'http://localhost:3000', 'X-CSRF-Protection': '1'}
    if headers: h.update(headers)
    if body is not None: h['Content-Type'] = 'application/json'
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(a.base_url + path, data=data, headers=h, method=method)
    try:
        res = client.open(req, timeout=15)
    except urllib.error.HTTPError as error:
        res = error
    raw = res.read()
    assert res.status == expected, f'{method} {path}: expected {expected}, got {res.status}'
    print(f'{res.status} {method} {path}')
    return (json.loads(raw) if raw and raw[:1] in (b'{', b'[') else None), res.headers

for service in ('auth', 'posts', 'chats', 'notifications'):
    call('GET', f'/api/v1/{service}/health', 200)
call('GET', '/api/v1/postsXYZ', 404)
call('GET', '/api/v1/auth/me', 401)
call('POST', '/api/v1/posts', 401, {'content': 'unauthenticated'})
_, h = call('OPTIONS', '/api/v1/auth/login', 200, headers={'Access-Control-Request-Method': 'POST', 'Access-Control-Request-Headers': 'content-type,x-csrf-protection'})
assert h['Access-Control-Allow-Origin'] == 'http://localhost:3000'
call('GET', '/api/v1/auth/me', 403, headers={'Origin': 'https://untrusted.example'})
rid = str(uuid.uuid4())
_, h = call('GET', '/api/v1/auth/health', 200, headers={'X-Request-ID': rid})
assert h['X-Request-ID'] == rid
credentials = {'email': f'kong-smoke-{uuid.uuid4().hex[:12]}@example.com', 'password': secrets.token_urlsafe(24)}
call('POST', '/api/v1/auth/register', 201, credentials)
tokens, _ = call('POST', '/api/v1/auth/login', 200, credentials)
auth = {'Authorization': 'Bearer ' + tokens['data']['access_token']}
user, _ = call('GET', '/api/v1/auth/me', 200, headers=auth)
post, _ = call('POST', '/api/v1/posts', 201, {'content': 'Kong gateway verification'}, auth)
post_id = post['data']['id']
try:
    call('GET', '/api/v1/posts', 200)
    call('GET', '/api/v1/posts/' + post_id, 200)
    call('GET', '/api/v1/users/' + user['data']['id'] + '/posts', 200)
    call('PATCH', '/api/v1/posts/' + post_id, 200, {'content': 'Kong verified'}, auth)
finally:
    call('DELETE', '/api/v1/posts/' + post_id, 204, headers=auth)
refreshed, _ = call('POST', '/api/v1/auth/refresh', 200, headers={'X-CSRF-Token': tokens['data']['csrf_token']})
call('GET', '/api/v1/auth/me', 200, headers={'Authorization': 'Bearer ' + refreshed['data']['access_token']})
call('POST', '/api/v1/auth/logout', 204, headers={'X-CSRF-Token': refreshed['data']['csrf_token']})
call('POST', '/api/v1/posts', 413, {'content': 'x' * 17000})
print('Gateway smoke test passed. Test account retained:', credentials['email'])
