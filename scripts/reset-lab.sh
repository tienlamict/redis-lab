#!/usr/bin/env bash
# Reset a single Redis lab: uninstall chart, wipe PVCs, reinstall.
# Usage: reset-lab.sh standalone|sentinel|cluster
set -euo pipefail

LAB="${1:?usage: reset-lab.sh standalone|sentinel|cluster}"
NS="redis-${LAB}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

helm uninstall "${NS}" -n "${NS}" 2>/dev/null || true
kubectl delete pvc --all -n "${NS}" --ignore-not-found=true 2>/dev/null || true
sleep 3
"${SCRIPT_DIR}/deploy-lab.sh" "${LAB}"
