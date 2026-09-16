#!/usr/bin/env bash
set -Eeuo pipefail
exec kubectl --context kind-social-media -n social-media-dev port-forward svc/kong-gateway 8000:8000
