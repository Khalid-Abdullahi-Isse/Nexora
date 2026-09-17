#!/usr/bin/env bash
# Compatibility entry point; shares lifecycle and locking with start-dev.sh.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
umask 077
mkdir -p .secrets/dev
exec 9>.secrets/dev/workflow.lock
flock -n 9 || { echo 'Another development start/stop is running.' >&2; exit 1; }
python3 scripts/dev-forward.py start
