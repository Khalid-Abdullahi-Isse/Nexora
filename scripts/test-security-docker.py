#!/usr/bin/env python3
"""Check the built images with disposable Compose volumes/network and random ports."""
import json, os, secrets, socket, subprocess, tempfile, time
from pathlib import Path
from urllib.request import Request,urlopen
ROOT=Path(__file__).resolve().parents[1]
def port():
    with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def main():
    env=os.environ.copy()
    for key in ['POSTGRES_PASSWORD','POSTGRES_ADMIN_PASSWORD','POST_DATABASE_PASSWORD','CHAT_DATABASE_PASSWORD','NOTIFICATION_DATABASE_PASSWORD','REDIS_PASSWORD']:env[key]=secrets.token_urlsafe(32)
    env.update(LOCAL_UID=str(os.getuid()),LOCAL_GID=str(os.getgid()),JWT_ISSUER='social-media-auth',JWT_KEY_ID='development-1',ALLOWED_ORIGINS='http://localhost:3000')
    with tempfile.TemporaryDirectory(prefix='social-security-docker-') as folder:
        tmp=Path(folder);name='security-smoke-'+secrets.token_hex(5)
        subprocess.run(['go','run','./shared/cmd/security-keys',str(tmp/'keys')],cwd=ROOT,check=True)
        raw=subprocess.check_output(['docker','compose','-f','docker/docker-compose.yml','config','--format','json'],cwd=ROOT,env=env)
        config=json.loads(raw);config['name']=name
        for key,v in config['volumes'].items():v['name']=name+'-'+key
        for key,v in config['networks'].items():v['name']=name+'-'+key
        ports={}
        for service,settings in config['services'].items():
            settings.pop('container_name',None)
            if 'build' in settings:
                settings.pop('build');settings['image']='social-media-backend-'+service+':latest';settings['pull_policy']='never'
            for item in settings.get('ports',[]):
                assigned=port();item['published']=str(assigned);item['host_ip']='127.0.0.1';ports[service]=assigned
            for volume in settings.get('volumes',[]):
                if volume.get('target')=='/keys/private.pem':volume['source']=str(tmp/'keys/private.pem')
                if volume.get('target')=='/keys/public-keys.json':volume['source']=str(tmp/'keys/public-keys.json')
        path=tmp/'compose.json';path.write_text(json.dumps(config));path.chmod(0o600)
        command=['docker','compose','-f',str(path),'-p',name]
        logfile=tmp/'startup.log'
        try:
            with logfile.open('w') as log:
                result=subprocess.run(command+['up','-d','--no-build'],stdout=log,stderr=subprocess.STDOUT)
            if result.returncode:
                raise RuntimeError('isolated Docker startup failed: '+''.join(logfile.read_text().splitlines(True)[-12:]))
            for service in ['auth-service','post-service','chat-service','notification-service']:
                for attempt in range(100):
                    try:
                        with urlopen(f'http://127.0.0.1:{ports[service]}/health',timeout=2) as r:
                            if r.status==200:break
                    except Exception:time.sleep(.2)
                else:raise RuntimeError(service+' failed health check')
            body=json.dumps({'email':'docker-smoke@example.com','password':'docker-smoke-password-sentinel'}).encode()
            for action,expected in [('register',201),('login',200)]:
                r=Request(f'http://127.0.0.1:{ports["auth-service"]}/api/v1/auth/{action}',data=body,headers={'Content-Type':'application/json','Origin':'http://localhost:3000','X-CSRF-Protection':'1'},method='POST')
                with urlopen(r,timeout=10) as response:
                    assert response.status==expected
                    data=json.load(response)
                    if action=='login':assert data['data']['expires_in']==900 and 'HttpOnly' in response.headers['Set-Cookie']
            logs=subprocess.check_output(command+['logs','--no-color','auth-service','post-service','chat-service','notification-service'],stderr=subprocess.STDOUT).decode()
            for secret in [env[k] for k in ['POSTGRES_PASSWORD','POSTGRES_ADMIN_PASSWORD','POST_DATABASE_PASSWORD','CHAT_DATABASE_PASSWORD','NOTIFICATION_DATABASE_PASSWORD','REDIS_PASSWORD']]+['docker-smoke-password-sentinel',data['data']['access_token']]:
                assert secret not in logs,'secret leaked in application logs'
            for service in ['auth-service','post-service','chat-service','notification-service']:
                uid=subprocess.check_output(command+['exec','-T',service,'id','-u']).decode().strip();assert uid!='0','root runtime'
                if service!='auth-service':
                    result=subprocess.run(command+['exec','-T',service,'test','-e','/keys/private.pem'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                    assert result.returncode!=0,'private key mounted in verifier service'
            print('PASS: Docker initialization, migrations, four health checks, registration/login, non-root users, public/private key isolation and log redaction')
        finally:
            # Only resources named for this newly generated isolated project are removed.
            subprocess.run(command+['down','-v','--remove-orphans'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
if __name__=='__main__':main()
