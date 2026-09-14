#!/usr/bin/env bash
# Install the controller locally; application sync requires the chart to be in Git.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
CONTEXT=kind-social-media
VERSION=v3.5.2
for command in kubectl curl python3; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
k() { kubectl --context "$CONTEXT" "$@"; }
k cluster-info >/dev/null
manifest="$(mktemp)"
trap 'rm -f "$manifest"' EXIT
curl --fail --silent --show-error --location \
  "https://raw.githubusercontent.com/argoproj/argo-cd/$VERSION/manifests/install.yaml" \
  --output "$manifest"
k create namespace argocd --dry-run=client -o yaml | k apply -f -
k apply --server-side -n argocd -f "$manifest"
# Pinned versions can reuse kind's local cache instead of checking registries on every start.
python3 - <<'PYTHON'
import json, subprocess
k = ['kubectl', '--context', 'kind-social-media', '-n', 'argocd']
for kind, name in [('deployment', 'argocd-' + n) for n in (
        'redis', 'repo-server', 'server', 'dex-server', 'applicationset-controller',
        'notifications-controller')] + [('statefulset', 'argocd-application-controller')]:
    obj = json.loads(subprocess.check_output(k + ['get', kind, name, '-o', 'json']))
    pod = obj['spec']['template']['spec']
    patch = {field: [{'name': c['name'], 'imagePullPolicy': 'IfNotPresent'} for c in pod[field]]
             for field in ('containers', 'initContainers') if field in pod}
    subprocess.run(k + ['patch', kind, name, '--type=strategic', '-p',
                        json.dumps({'spec': {'template': {'spec': patch}}})], check=True)
PYTHON
for resource in deployment/argocd-redis deployment/argocd-repo-server deployment/argocd-server \
  deployment/argocd-dex-server deployment/argocd-applicationset-controller \
  deployment/argocd-notifications-controller statefulset/argocd-application-controller; do
  k -n argocd rollout status "$resource" --timeout=10m
done
k apply -f "$ROOT/deployments/argocd/project.yaml"
k -n argocd get pods,services
printf '\nArgoCD is installed. Publish the chart to the configured Git revision before applying the Application.\n'
