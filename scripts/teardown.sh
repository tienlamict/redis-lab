#!/usr/bin/env bash
# Uninstall all labs and delete the Kind cluster.
set -euo pipefail

for LAB in standalone sentinel cluster; do
  NS="redis-${LAB}"
  helm uninstall "${NS}" -n "${NS}" 2>/dev/null || true
  kubectl delete namespace "${NS}" --ignore-not-found=true 2>/dev/null || true
done

kind delete cluster --name redis-lab 2>/dev/null || true
