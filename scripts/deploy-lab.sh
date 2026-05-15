#!/usr/bin/env bash
# Deploy a single Redis lab via Bitnami Helm chart.
# Usage: deploy-lab.sh standalone|sentinel|cluster
set -euo pipefail

LAB="${1:?usage: deploy-lab.sh standalone|sentinel|cluster}"
NS="redis-${LAB}"

case "${LAB}" in
  standalone|sentinel)
    CHART="bitnami/redis"
    ;;
  cluster)
    CHART="bitnami/redis-cluster"
    ;;
  *)
    echo "unknown lab: ${LAB}" >&2
    exit 1
    ;;
esac

# Resolve script dir so we can run from anywhere.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VALUES_FILE="${REPO_ROOT}/helm/values-${LAB}.yaml"

helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
helm repo update bitnami >/dev/null

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install "${NS}" "${CHART}" \
  --namespace "${NS}" \
  --values "${VALUES_FILE}" \
  --wait --timeout 5m

echo
echo "=== Pods in ${NS} ==="
kubectl get pods -n "${NS}" -o wide
echo
echo "=== Services in ${NS} ==="
kubectl get svc -n "${NS}"
