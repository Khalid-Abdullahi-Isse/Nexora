#!/usr/bin/env python3
"""Refuse competing owners before building images or modifying Secrets."""
import json
import subprocess

K = ['kubectl', '--context', 'kind-social-media']
def get(*args):
    return json.loads(subprocess.check_output(K + list(args) + ['-o', 'json']))

crds = get('get', 'crd')
if any(x['metadata']['name'] == 'applications.argoproj.io' for x in crds['items']):
    for app in get('get', 'applications.argoproj.io', '-A')['items']:
        dest = app['spec']['destination']
        if dest.get('namespace') == 'social-media' and dest.get('server', 'https://kubernetes.default.svc') == 'https://kubernetes.default.svc':
            raise SystemExit('ArgoCD application ' + app['metadata']['name'] + ' targets social-media. Use GitOps or move its destination before using manual Helm.')
for item in get('get', 'deploy,sts,svc,pvc,cm,sa', '-n', 'social-media')['items']:
    m = item['metadata']
    if m['name'] in ('default', 'kube-root-ca.crt'):
        continue
    if m.get('annotations', {}).get('argocd.argoproj.io/tracking-id'):
        raise SystemExit('ArgoCD owns ' + m['name'] + '; resolve ownership before deploying.')
    owner = m.get('annotations', {}).get('meta.helm.sh/release-name')
    if owner and owner != 'nexora':
        raise SystemExit('Another Helm release owns ' + m['name'] + ': ' + owner)
    if not owner and item['kind'] in ('Deployment', 'StatefulSet', 'Service', 'PersistentVolumeClaim'):
        raise SystemExit('Unmanaged resource ' + m['name'] + ' requires reviewed adoption. See docs/deployment.md; startup never deletes or takes ownership automatically.')
print('Ownership check passed: manual Helm nexora / social-media.')
