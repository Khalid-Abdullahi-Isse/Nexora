#!/usr/bin/env bash
set -uo pipefail
NS="${1:-social-media}"
k() { kubectl --context kind-social-media "$@"; }
command -v kubectl >/dev/null || { echo 'kubectl is required.' >&2; exit 1; }
status=0
k get nodes || status=1
for resource in pods services deployments statefulsets jobs pvc; do
  k get "$resource" -n "$NS" || status=1
done
exit "$status"
