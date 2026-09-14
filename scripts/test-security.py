#!/usr/bin/env python3
"""Run security tests and HTTP smoke checks against disposable local services.
Requires Go, PostgreSQL binaries, Redis and openssl. Never uses project DB/volumes.
"""
import base64, hashlib, json, os, secrets, socket, subprocess, tempfile, time
from pathlib import Path
from urllib.request import Request, urlopen
from urllib.error import HTTPError
ROOT=Path(__file__).resolve().parents[1]
MODULES=['shared','services/auth-service','services/post-service','services/chat-service','services/notification-service']
def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0)); return s.getsockname()[1]
def run(args, env, **kw):
    return subprocess.run(args,cwd=ROOT,env=env,check=True,**kw)
def main():
    with tempfile.TemporaryDirectory(prefix='social-security-') as temp:
        tmp=Path(temp); password=secrets.token_urlsafe(32); redis_password=secrets.token_urlsafe(32)
        pgport,redisport=port(),port(); procs=[]; handles=[]; started=False
        env=os.environ.copy()
        # Explicit environment isolates tests from shell/project configuration.
        env.update(APP_ENV='test',ENV_FILE='/dev/null',POSTGRES_HOST='127.0.0.1',POSTGRES_PORT=str(pgport),POSTGRES_USER=os.environ.get('USER','postgres'),POSTGRES_PASSWORD=password,POSTGRES_DB='security_test',POSTGRES_SSLMODE='disable',REDIS_ADDR=f'redis://:{redis_password}@127.0.0.1:{redisport}/0',RATE_LIMIT_ENABLED='true',TRUSTED_PROXIES='',JWT_ISSUER='security-test',JWT_KEY_ID='development-1',JWT_PRIVATE_KEY_FILE=str(tmp/'keys/private.pem'),JWT_PUBLIC_KEYS_FILE=str(tmp/'keys/public-keys.json'),ALLOWED_ORIGINS='http://localhost:3000',AUTH_COOKIE_INSECURE='true',REFRESH_TOKEN_TTL='168h',DATABASE_URL='')
        pw=tmp/'pg-password';pw.write_text(password);pw.chmod(0o600)
        try:
            run(['initdb','-D',str(tmp/'pg'),'--auth-local=trust','--auth-host=scram-sha-256','--pwfile',str(pw)],env,stdout=subprocess.DEVNULL)
            run(['pg_ctl','-D',str(tmp/'pg'),'-l',str(tmp/'postgres.log'),'-o',f'-h 127.0.0.1 -p {pgport} -k {tmp}','-w','start'],env,stdout=subprocess.DEVNULL);started=True
            pgenv=dict(env,PGPASSWORD=password)
            run(['psql','-X','-v','ON_ERROR_STOP=1','-h','127.0.0.1','-p',str(pgport),'-U',env['POSTGRES_USER'],'-d','postgres','-c','CREATE DATABASE security_test'],pgenv,stdout=subprocess.DEVNULL)
            # Provision test-only distinct runtime identities, without command-line secrets.
            roleenv=dict(pgenv,POSTGRES_USER=env['POSTGRES_USER'],POSTGRES_DB='security_test',PGHOST='127.0.0.1',PGPORT=str(pgport),AUTH_DATABASE_PASSWORD=password,POST_DATABASE_PASSWORD=password,CHAT_DATABASE_PASSWORD=password,NOTIFICATION_DATABASE_PASSWORD=password)
            run(['sh','docker/postgres-init.sh'],roleenv,stdout=subprocess.DEVNULL)
            run(['go','run','./shared/cmd/migrate','all','up'],env)
            run(['go','run','./shared/cmd/security-keys',str(tmp/'keys')],env)
            privilege_sql="SELECT has_table_privilege('app_post','users','SELECT'),has_table_privilege('app_chat','users','UPDATE'),has_table_privilege('app_notification','sessions','SELECT'),has_table_privilege('app_auth','security_audit','DELETE'),has_table_privilege('app_auth','users','INSERT')"
            result=run(['psql','-X','-At','-h','127.0.0.1','-p',str(pgport),'-U',env['POSTGRES_USER'],'-d','security_test','-c',privilege_sql],pgenv,capture_output=True,text=True)
            assert result.stdout.strip()=='f|f|f|f|t','runtime grants crossed service or audit boundaries'

            redisconf=tmp/'redis.conf';redisconf.write_text(f'bind 127.0.0.1\nport {redisport}\nsave ""\nappendonly no\nrequirepass {redis_password}\n');redisconf.chmod(0o600)
            logfile=(tmp/'redis.log').open('w');handles.append(logfile);procs.append(subprocess.Popen(['redis-server',str(redisconf)],stdout=logfile,stderr=subprocess.STDOUT))
            dsn=f'postgres://{env["POSTGRES_USER"]}:{password}@127.0.0.1:{pgport}/security_test?sslmode=disable'
            env.update(RESOURCE_TEST_DATABASE_URL=dsn,AUTH_TEST_DATABASE_URL=dsn,MIGRATION_TEST_DATABASE_URL=dsn,MIGRATION_SOURCE_ROOT=str(ROOT))
            run(['go','test','-race','-count=1',*[m+'/...' if m.startswith('./') else './'+m+'/...' for m in MODULES]],env)
            run(['go','vet',*['./'+m+'/...' for m in MODULES]],env)
            ports={}
            for service in ['auth','post','chat','notification']:
                binary=tmp/service
                run(['go','build','-o',str(binary),'./services/'+service+'-service/cmd/server'],env)
                serverport=port();ports[service]=serverport
                appenv=dict(env,POSTGRES_USER='app_'+service)
                appenv[service.upper()+'_SERVICE_PORT']=str(serverport)
                logfile=(tmp/(service+'.log')).open('w');handles.append(logfile)
                procs.append(subprocess.Popen([str(binary)],cwd=ROOT,env=appenv,stdout=logfile,stderr=subprocess.STDOUT))
            def req(method,path,data=None,token=None,cookie=None,csrf=None,origin='http://localhost:3000'):
                headers={'Content-Type':'application/json','X-CSRF-Protection':'1'}
                if origin:headers['Origin']=origin
                if token:headers['Authorization']='Bearer '+token
                if cookie:headers['Cookie']=cookie
                if csrf:headers['X-CSRF-Token']=csrf
                body=json.dumps(data).encode() if data is not None else None
                r=Request(f'http://127.0.0.1:{ports["auth"]}'+path,data=body,headers=headers,method=method)
                try:response=urlopen(r,timeout=10)
                except HTTPError as e:response=e
                raw=response.read();result=json.loads(raw) if raw else None
                return response.status,result,response.headers
            for service,serverport in ports.items():
                ready=False
                for _ in range(100):
                    try:
                        with urlopen(f'http://127.0.0.1:{serverport}/health',timeout=1) as response:
                            if response.status==200:ready=True;break
                    except Exception:time.sleep(.1)
                if not ready:raise AssertionError(service+' failed startup; isolated logs retained only for this run')
            def expect(code,result):
                assert result[0]==code, f'expected {code}, received {result[0]}'
                return result
            user={'email':'http-smoke@example.com','password':'smoke-password-sentinel-123'}
            expect(201,req('POST','/api/v1/auth/register',user))
            status,body,headers=expect(200,req('POST','/api/v1/auth/login',user))
            data=body['data'];access=data['access_token'];csrf=data['csrf_token'];cookie=headers['Set-Cookie'].split(';')[0]
            assert 'HttpOnly' in headers['Set-Cookie'] and 'SameSite=Lax' in headers['Set-Cookie']
            assert 'refresh_token' not in data and headers['Cache-Control']=='no-store'
            claims=json.loads(base64.urlsafe_b64decode(access.split('.')[1]+'=='))
            assert claims['exp']-claims['iat']==900
            expect(200,req('GET','/api/v1/auth/me',token=access))
            _,bootstrap,_=expect(200,req('POST','/api/v1/auth/csrf',cookie=cookie))
            assert bootstrap['data']['csrf_token']==csrf,'session cannot resume after reload'
            expect(403,req('POST','/api/v1/auth/csrf',cookie=cookie,origin='https://evil.example'))
            expect(401,req('GET','/api/v1/auth/me'))
            expect(401,req('GET','/api/v1/auth/me',token=access[:-8]+'forgedxx'))
            expect(403,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf=csrf,origin='https://evil.example'))
            expect(403,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf='wrong'))
            _,body,headers=expect(200,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf=csrf))
            rotated=body['data'];newcookie=headers['Set-Cookie'].split(';')[0]
            expect(200,req('GET','/api/v1/auth/me',token=rotated['access_token']))
            expect(401,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf=csrf))
            expect(401,req('POST','/api/v1/auth/refresh',cookie=newcookie,csrf=rotated['csrf_token']))
            _,body,headers=expect(200,req('POST','/api/v1/auth/login',user));second=body['data'];cookie=headers['Set-Cookie'].split(';')[0]
            expect(204,req('POST','/api/v1/auth/logout',cookie=cookie,csrf=second['csrf_token']))
            expect(401,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf=second['csrf_token']))
            _,body,headers=expect(200,req('POST','/api/v1/auth/login',user));third=body['data'];cookie=headers['Set-Cookie'].split(';')[0]
            expect(204,req('POST','/api/v1/auth/logout-all',token=third['access_token']))
            expect(401,req('POST','/api/v1/auth/refresh',cookie=cookie,csrf=third['csrf_token']))
            # Normal user is denied before target UUIDs are inspected.
            expect(403,req('PUT','/api/v1/auth/admin/users/00000000-0000-4000-8000-000000000001/roles/00000000-0000-4000-8000-000000000002',token=third['access_token']))
            got429=False
            for _ in range(12):
                status,_,_=req('POST','/api/v1/auth/login',{'email':user['email'],'password':'incorrect-password'})
                if status==429:got429=True;break
            assert got429,'login was not rate limited'
            got429=False
            for _ in range(35):
                status,_,_=req('POST','/api/v1/auth/refresh')
                if status==429:got429=True;break
            assert got429,'refresh was not rate limited'
            for handle in handles:handle.flush()
            logtext=''.join((tmp/(name+'.log')).read_text() for name in ['auth','post','chat','notification'])
            for sensitive in [password,redis_password,user['password'],access,cookie.split('=',1)[1]]:
                assert sensitive not in logtext,'sensitive value appeared in service logs'
            assert 'BEGIN RSA PRIVATE KEY' not in logtext and '$2a$' not in logtext
            print('PASS: isolated PostgreSQL/Redis, race tests, vet, four-service startup, HTTP auth/rotation/replay/logout/RBAC/rate-limit checks and log redaction')
        finally:
            for proc in reversed(procs):
                proc.terminate()
                try:proc.wait(timeout=8)
                except subprocess.TimeoutExpired:proc.kill();proc.wait()
            for handle in handles:handle.close()
            if started:subprocess.run(['pg_ctl','-D',str(tmp/'pg'),'-m','fast','-w','stop'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
if __name__=='__main__':main()
