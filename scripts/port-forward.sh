#!/usr/bin/env bash
# Port-forward Redis services from the Kind cluster to host:
#   localhost:6379  -> redis-standalone (master)
#   localhost:26379 -> redis-sentinel sentinel port
#   localhost:6381  -> redis-cluster
#
# Bitnami chart service names differ across chart versions, so we kubectl-get
# each namespace and pick the first matching service.
set -euo pipefail

# Pick the first service name matching a regex in a namespace; falls back to
# the second arg if nothing matches.
pick_svc() {
  local ns="$1" pattern="$2" fallback="$3"
  local found
  found="$(kubectl get svc -n "${ns}" -o name 2>/dev/null \
            | sed 's|service/||' \
            | grep -E "${pattern}" \
            | head -1 || true)"
  echo "${found:-${fallback}}"
}

pf() {
  local ns="$1" svc="$2" local_port="$3" target_port="$4"
  echo "[pf] -n ${ns} svc/${svc} ${local_port}:${target_port}"
  kubectl port-forward -n "${ns}" "svc/${svc}" "${local_port}:${target_port}" &
}

STANDALONE_SVC="$(pick_svc redis-standalone '^redis-standalone(-master)?$' redis-standalone-master)"
SENTINEL_SVC="$(pick_svc redis-sentinel '^redis-sentinel$' redis-sentinel)"
CLUSTER_SVC="$(pick_svc redis-cluster '^redis-cluster$' redis-cluster)"

pf redis-standalone "${STANDALONE_SVC}" 6379 6379
pf redis-sentinel   "${SENTINEL_SVC}"   26379 26379
pf redis-cluster    "${CLUSTER_SVC}"    6381 6379

trap 'kill 0' EXIT INT TERM
echo "[pf] waiting on background port-forwards (ctrl-c to stop)"
wait
