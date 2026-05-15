#!/usr/bin/env bash
# Port-forward Redis services from Kind cluster to host.
# NOTE: Bitnami service names may differ between chart versions.
# This script auto-detects the correct service name per namespace.
set -euo pipefail

pf() {
  local ns="$1" svc="$2" local_port="$3" target_port="$4"
  echo "port-forward -n ${ns} svc/${svc} ${local_port}:${target_port}"
  kubectl port-forward -n "${ns}" "svc/${svc}" "${local_port}:${target_port}" &
}

# Standalone: master service (writes) — Bitnami names it <release>-master.
STANDALONE_SVC="$(kubectl get svc -n redis-standalone -o name | grep -E 'redis-standalone(-master)?$' | head -1 | sed 's|service/||')"
pf redis-standalone "${STANDALONE_SVC:-redis-standalone-master}" 6379 6379

# Sentinel: Bitnami exposes redis on 6379 and sentinel on 26379 via the same svc.
SENTINEL_SVC="$(kubectl get svc -n redis-sentinel -o name | grep -E 'redis-sentinel$' | head -1 | sed 's|service/||')"
pf redis-sentinel "${SENTINEL_SVC:-redis-sentinel}" 6380 6379
pf redis-sentinel "${SENTINEL_SVC:-redis-sentinel}" 26379 26379

# Cluster: Bitnami exposes redis on 6379 via the headless or main svc.
CLUSTER_SVC="$(kubectl get svc -n redis-cluster -o name | grep -E 'redis-cluster$' | head -1 | sed 's|service/||')"
pf redis-cluster "${CLUSTER_SVC:-redis-cluster}" 6381 6379

trap 'kill 0' EXIT INT TERM
wait
