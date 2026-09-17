#!/usr/bin/env bash
# Local development only. Always target this cluster explicitly.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=docker
CLUSTER=social-media
CONTEXT=kind-social-media
NS=social-media
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
k cluster-info
k wait --for=condition=Ready node --all --timeout=180s
python3 scripts/dev-preflight.py
python3 -c "import yaml" || fail 'Python PyYAML is required.'

for service in auth post chat notification migrate; do
  dockerfile="docker/$service-service.Dockerfile"
  [[ "$service" != migrate ]] || dockerfile=docker/migrate.Dockerfile
  docker build -f "$dockerfile" -t "social-$service:latest" .
done
load_image() {
  # Docker's containerd store can export an OCI index with unpulled platforms.
  # Import the host platform explicitly instead of kind's --all-platforms.
  local arch
  arch="$(docker version --format '{{.Server.Arch}}')"
  while read -r node; do
    docker save "$1" | docker exec -i "$node" ctr -n k8s.io images import --platform "linux/$arch" -
  done < <(kind get nodes --name "$CLUSTER")
}
for service in auth post chat notification migrate; do
  load_image "social-$service:latest"
done
KONG_IMAGE="$(python3 -c 'import yaml; print(yaml.safe_load(open("deployments/helm/social-media-backend/values.yaml"))["kong"]["image"])')"
docker pull "$KONG_IMAGE"
load_image "$KONG_IMAGE"

# Existing Secrets are authoritative: never replace live database credentials.
credentials="$(k -n "$NS" get secret social-media-backend-secrets --ignore-not-found -o name)"
jwt="$(k -n "$NS" get secret jwt-keys --ignore-not-found -o name)"
if [[ -z "$credentials" || -z "$jwt" ]]; then
  [[ -z "$credentials" && -z "$jwt" ]] || fail 'Only one required Secret exists; restore the missing Secret before deploying.'
  claims="$(k -n "$NS" get pvc --no-headers 2>/dev/null)"
  [[ -z "$claims" ]] || fail 'Existing PVCs found without credentials. Restore matching Secrets; do not generate new passwords.'
# Generate once, reuse on subsequent deployments. Never source .env as shell code.
# These independent kind credentials do not modify the Compose development setup.
if [[ ! -f .secrets/kind/social-media/credentials.env ]]; then
  existing_secret="$(k -n "$NS" get secret social-media-backend-secrets --ignore-not-found -o name)"
  [[ -z "$existing_secret" ]] || fail 'Restore .secrets/kind/social-media/credentials.env: cluster credentials already exist.'
fi
umask 077
python3 - <<'PY'
from pathlib import Path
import json, os, secrets, subprocess
from urllib.parse import quote
p = Path('.secrets/kind/social-media')
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
        raise SystemExit('Private key missing; restore .secrets/kind/social-media/private.pem before deploying.')
    subprocess.run(['openssl', 'genrsa', '-out', str(private), '3072'], check=True, stdout=subprocess.DEVNULL)
public = subprocess.check_output(['openssl', 'rsa', '-in', str(private), '-pubout'], stderr=subprocess.DEVNULL).decode()
(p / 'public-keys.json').write_text(json.dumps({'development-1': public}))
for file in p.iterdir():
    if file.is_file(): os.chmod(file, 0o600)
PY
k create namespace "$NS" --dry-run=client -o yaml | k apply -f -
k -n "$NS" create secret generic social-media-backend-secrets --from-env-file=.secrets/kind/social-media/credentials.env --dry-run=client -o yaml | k apply -f -
k -n "$NS" create secret generic jwt-keys --from-file=.secrets/kind/social-media/public-keys.json --from-file=.secrets/kind/social-media/private.pem --dry-run=client -o yaml | k apply -f -
fi
helm upgrade --install nexora deployments/helm/social-media-backend \
  --kube-context "$CONTEXT" --namespace "$NS" --create-namespace \
  -f deployments/helm/social-media-backend/values-dev.yaml \
  --set-string "rolloutVersion=$(date -u +%Y%m%dT%H%M%SZ)" \
  --timeout 10m
# Hooks complete before rollout checks; --wait can deadlock a fresh database
# when application readiness requires schema installed by a post-install hook.
for workload in statefulset/postgres statefulset/redis deployment/auth-service deployment/post-service deployment/chat-service deployment/notification-service deployment/kong-gateway; do
  k -n "$NS" rollout status "$workload" --timeout=300s
done
k -n "$NS" get pods,services,jobs,pvc
