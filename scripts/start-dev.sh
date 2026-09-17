#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$HOME/.local/bin:$PATH"
for tool in python3 flock; do
  command -v "$tool" >/dev/null || { echo "Error: $tool is required." >&2; exit 1; }
done
umask 077
mkdir -p .secrets/dev
exec 9>.secrets/dev/workflow.lock
flock -n 9 || { echo 'Another development start/stop is running.' >&2; exit 1; }
trap 'echo "Startup failed. Inspect ./scripts/kind-status.sh and .secrets/dev/forward.log." >&2' ERR
echo '[1/6] Checking localhost:8000...'
python3 scripts/dev-forward.py check
echo '[2/6] Checking cluster, building images, and deploying Helm chart...'
./scripts/kind-deploy.sh
echo '[3/6] Deployments and databases are ready.'
echo '[4/6] Checking pods and services...'
kubectl --context kind-social-media -n social-media get pods,svc
echo '[5/6] Checking Kong...'
kubectl --context kind-social-media -n social-media get svc kong-gateway
echo '[6/6] Exposing Kong on localhost:8000...'
python3 scripts/dev-forward.py start
