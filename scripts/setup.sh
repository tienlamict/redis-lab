#!/usr/bin/env bash
# One-shot Phase 1 setup: create Kind cluster and deploy all 3 labs.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if ! kind get clusters | grep -q '^redis-lab$'; then
  kind create cluster --config "${REPO_ROOT}/kind/cluster.yaml"
else
  echo "Kind cluster 'redis-lab' already exists, skipping create."
fi

"${SCRIPT_DIR}/deploy-lab.sh" standalone
"${SCRIPT_DIR}/deploy-lab.sh" sentinel
"${SCRIPT_DIR}/deploy-lab.sh" cluster

echo
echo "=== All redis-* pods ==="
kubectl get pods -A | grep '^redis-' || true
