#!/usr/bin/env bash
# Local development only. Always target this cluster explicitly.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=docker
CLUSTER=social-media
CONTEXT=kind-social-media
NS=social-media-dev
k() { kubectl --context "$CONTEXT" "$@"; }
fail() { echo "Error: $*" >&2; exit 1; }
for command in docker kind kubectl helm python3 openssl; do
  command -v "$command" >/dev/null || fail "$command is required; install it and rerun this script."
done
docker info >/dev/null 2>&1 || fail 'Docker is not running or is inaccessible.'
trap 'echo "Deployment stopped. Run ./scripts/kind-status.sh; see docs/deployment.md for debug commands." >&2' ERR
clusters="$(kind get clusters)"
if ! grep -qx "$CLUSTER" <<< "$clusters"; then
  kind create cluster --name "$CLUSTER" --wait 120s
else
  echo "Reusing existing kind cluster: $CLUSTER"
  # Recover stopped nodes without deleting the cluster or its persistent data.
  while read -r node; do
    [[ -z "$node" ]] || docker start "$node" >/dev/null
  done < <(docker ps -aq --filter "label=io.x-k8s.kind.cluster=$CLUSTER" --filter status=exited)
fi
kind export kubeconfig --name "$CLUSTER" >/dev/null
k wait --for=condition=Ready node --all --timeout=180s

for service in auth post chat notification migrate; do
  dockerfile="docker/$service-service.Dockerfile"
  [[ "$service" != migrate ]] || dockerfile=docker/migrate.Dockerfile
  docker build -f "$dockerfile" -t "social-$service:latest" .
done
for service in auth post chat notification migrate; do
  kind load docker-image "social-$service:latest" --name "$CLUSTER"
done

# Generate once, reuse on subsequent deployments. Never source .env as shell code.
# These independent kind credentials do not modify the Compose development setup.
if [[ ! -f .secrets/kind/credentials.env ]]; then
  existing_secret="$(k -n "$NS" get secret social-media-backend-secrets --ignore-not-found -o name)"
  [[ -z "$existing_secret" ]] || fail 'Restore .secrets/kind/credentials.env: cluster credentials already exist.'
fi
umask 077
python3 - <<'PY'
from pathlib import Path
import json, os, secrets, subprocess
from urllib.parse import quote
p = Path('.secrets/kind')
p.mkdir(parents=True, exist_ok=True, mode=0o700)
env = p / 'credentials.env'
if not env.exists():
    values = {key: secrets.token_urlsafe(32) for key in (
        'POSTGRES_ADMIN_PASSWORD', 'AUTH_DATABASE_PASSWORD', 'POST_DATABASE_PASSWORD',
        'CHAT_DATABASE_PASSWORD', 'NOTIFICATION_DATABASE_PASSWORD', 'REDIS_PASSWORD')}
    values['REDIS_ADDR'] = 'redis://:' + quote(values['REDIS_PASSWORD'], safe='') + '@redis:6379/0'
    with env.open('x') as f:
        f.write(''.join(k + '=' + v + '\n' for k, v in values.items()))
private = p / 'private.pem'
if not private.exists():
    if (p / 'public-keys.json').exists():
        raise SystemExit('Private key missing; restore .secrets/kind/private.pem before deploying.')
    subprocess.run(['openssl', 'genrsa', '-out', str(private), '3072'], check=True, stdout=subprocess.DEVNULL)
public = subprocess.check_output(['openssl', 'rsa', '-in', str(private), '-pubout'], stderr=subprocess.DEVNULL).decode()
(p / 'public-keys.json').write_text(json.dumps({'development-1': public}))
for file in p.iterdir():
    if file.is_file(): os.chmod(file, 0o600)
PY
k create namespace "$NS" --dry-run=client -o yaml | k apply -f -
k -n "$NS" create secret generic social-media-backend-secrets --from-env-file=.secrets/kind/credentials.env --dry-run=client -o yaml | k apply -f -
k -n "$NS" create secret generic jwt-keys --from-file=.secrets/kind/public-keys.json --from-file=.secrets/kind/private.pem --dry-run=client -o yaml | k apply -f -
helm upgrade --install social-media deployments/helm/social-media-backend \
  --kube-context "$CONTEXT" --namespace "$NS" --create-namespace \
  -f deployments/helm/social-media-backend/values-dev.yaml \
  --set-string "rolloutVersion=$(date -u +%Y%m%dT%H%M%SZ)" \
  --wait --timeout 10m
k -n "$NS" get pods,services,jobs,pvc
