#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

if [[ "${CONFIRM_FAULT:-}" != "YES" ]]; then
  echo "Refusing to change network policy without CONFIRM_FAULT=YES." >&2
  exit 2
fi

NAMESPACE="${NAMESPACE:-shop}"
RELEASE="${RELEASE:-shop}"
CONTEXT="${KUBECTL_CONTEXT:-kind-lab5}"
FAULT_VALUES="${FAULT_VALUES:-lab-5/evidence/fault-values.yaml}"

for command in shuf helm kubectl; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

fault="$(shuf -e dns postgres-route postgres-port api-ingress -n 1)"
mkdir -p "$(dirname "$FAULT_VALUES")"

case "$fault" in
  dns)
    cat >"$FAULT_VALUES" <<'EOF'
networkPolicy:
  dns:
    enabled: false
EOF
    ;;
  postgres-route)
    cat >"$FAULT_VALUES" <<'EOF'
networkPolicy:
  postgres:
    enabled: false
EOF
    ;;
  postgres-port)
    cat >"$FAULT_VALUES" <<'EOF'
networkPolicy:
  ports:
    postgres: 6432
EOF
    ;;
  api-ingress)
    cat >"$FAULT_VALUES" <<'EOF'
networkPolicy:
  apiIngress:
    enabled: false
EOF
    ;;
esac

helm upgrade --install "$RELEASE" ./shop-chart \
  --kube-context "$CONTEXT" \
  --namespace "$NAMESPACE" \
  --create-namespace \
  -f shop-chart/values-lab5.yaml \
  -f "$FAULT_VALUES" \
  --wait=false

kubectl --context "$CONTEXT" -n "$NAMESPACE" rollout restart "deployment/${RELEASE}-api"
echo "A hidden connectivity fault was applied. Diagnose it with Hubble before reading $FAULT_VALUES."
