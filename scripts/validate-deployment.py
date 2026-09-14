#!/usr/bin/env python3
"""Render all chart environments and check deployment contracts (requires PyYAML)."""
from pathlib import Path
import subprocess
import yaml

ROOT = Path(__file__).resolve().parents[1]
CHART = ROOT / 'deployments/helm/social-media-backend'
assert (CHART / 'files/postgres-init.sh').read_bytes() == (ROOT / 'docker/postgres-init.sh').read_bytes(), 'Update chart initialization script to match Compose'
for env in ('default', 'dev', 'staging', 'production'):
    args = [] if env == 'default' else ['-f', str(CHART / f'values-{env}.yaml')]
    subprocess.run(['helm', 'lint', str(CHART), *args], check=True)
    for mode in ('helm', 'argocd'):
        namespace = 'social-media-' + env
        raw = subprocess.check_output(['helm', 'template', 'social-media', str(CHART), *args,
                                       '--namespace', namespace, '--set', 'deploymentMode=' + mode], text=True)
        docs = [d for d in yaml.safe_load_all(raw) if d]
        index = {(d['kind'], d['metadata']['name']): d for d in docs}
        assert len(index) == len(docs), 'Duplicate resource names'
        assert all(d['metadata']['namespace'] == namespace for d in docs)
        assert len([d for d in docs if d['kind'] == 'StatefulSet']) == 2
        pods = [d['spec']['template'] for d in docs if d['kind'] in ('Deployment', 'StatefulSet', 'Job')]
        for d in docs:
            if d['kind'] == 'Service':
                matches = [p for p in pods if all(p['metadata']['labels'].get(k) == v for k, v in d['spec']['selector'].items())]
                assert len(matches) == 1, d['metadata']['name']
                ports = matches[0]['spec']['containers'][0]['ports']
                assert any(p['name'] == d['spec']['ports'][0]['targetPort'] for p in ports)
            if d['kind'] in ('Deployment', 'StatefulSet'):
                assert all(d['spec']['template']['metadata']['labels'].get(k) == v for k, v in d['spec']['selector']['matchLabels'].items())
        for pod in pods:
            spec = pod['spec']
            assert not spec['automountServiceAccountToken']
            volumes = {v['name']: v for v in spec.get('volumes', [])}
            for v in volumes.values():
                if 'persistentVolumeClaim' in v:
                    assert ('PersistentVolumeClaim', v['persistentVolumeClaim']['claimName']) in index
                if 'configMap' in v:
                    assert ('ConfigMap', v['configMap']['name']) in index
                if 'secret' in v:
                    assert v['secret']['secretName'] in ('jwt-keys', 'postgres-tls', 'redis-tls')
            for c in spec.get('initContainers', []) + spec['containers']:
                assert c['securityContext']['allowPrivilegeEscalation'] is False
                for mount in c.get('volumeMounts', []):
                    assert mount['name'] in volumes
                for ref in c.get('envFrom', []):
                    assert ('ConfigMap', ref['configMapRef']['name']) in index
                for entry in c.get('env', []):
                    ref = entry.get('valueFrom', {}).get('secretKeyRef')
                    if ref: assert ref['name'] == 'social-media-backend-secrets'
            main = spec['containers'][0]
            if 'ports' in main:
                assert all(k in main for k in ('startupProbe', 'readinessProbe', 'livenessProbe'))
            if main['name'] in ('auth', 'post', 'chat', 'notification', 'migrate') and env == 'dev':
                assert main['imagePullPolicy'] == 'Never'
            if main['name'] in ('post', 'chat', 'notification'):
                assert volumes['keys']['secret']['items'] == [{'key': 'public-keys.json', 'path': 'public-keys.json'}]
        config = index['ConfigMap', 'social-media-backend-config']['data']
        assert config['POSTGRES_DB'] == 'social_media'
        assert config['POSTGRES_HOST'] == 'postgres'
        job = index['Job', 'social-media-backend-migrate']
        annotations = job['metadata']['annotations']
        if mode == 'argocd':
            assert annotations['argocd.argoproj.io/hook'] == 'Sync'
            assert int(annotations['argocd.argoproj.io/sync-wave']) == -1
            assert 'helm.sh/hook' not in annotations
        else:
            assert annotations['helm.sh/hook'] == 'post-install,post-upgrade'
        assert job['spec']['template']['spec']['containers'][0]['args'] == ['all', 'up']
        assert 'initContainers' in job['spec']['template']['spec']
        print(f'{env}/{mode}: {len(docs)} resources passed semantic checks')
# Invalid production tags and accidental additional databases must be rejected.
for overrides in (['--set', 'postgres.database=another_database'],
                  ['-f', str(CHART / 'values-production.yaml'), '--set', 'images.auth.tag=latest']):
    result = subprocess.run(['helm', 'template', 'social-media', str(CHART), *overrides], capture_output=True)
    assert result.returncode != 0, 'Expected invalid configuration to fail'
for file in (ROOT / 'deployments/argocd').glob('application-*.yaml'):
    app = yaml.safe_load(file.read_text())
    source = app['spec']['source']
    assert (ROOT / source['path']).is_dir()
    assert all((ROOT / source['path'] / f).is_file() for f in source['helm']['valueFiles'])
    assert app['spec']['project'] == 'social-media'
print('ArgoCD paths and negative configuration checks passed.')
