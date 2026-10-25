#!/usr/bin/env bash
set -euo pipefail

if [[ "${CONFIRM_CHAOS:-}" != "YES" ]]; then
  echo "Refusing to delete pods without CONFIRM_CHAOS=YES." >&2
  exit 2
fi

NAMESPACE="${NAMESPACE:-shop}"
RELEASE="${RELEASE:-shop}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-5}"
MAX_DELETIONS="${MAX_DELETIONS:-30}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-kind-lab5}"
selector="app.kubernetes.io/name=shop,app.kubernetes.io/instance=${RELEASE}"

for command in kubectl shuf python3; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

if [[ ! "$MAX_DELETIONS" =~ ^[1-9][0-9]*$ || ! "$INTERVAL_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
  echo "MAX_DELETIONS and INTERVAL_SECONDS must be positive integers." >&2
  exit 2
fi

deleted=0
empty_polls=0
while (( deleted < MAX_DELETIONS )); do
  pod_list="$(kubectl --context "$KUBECTL_CONTEXT" -n "$NAMESPACE" get pods \
    -l "$selector" -o json)"
  candidate_list="$(python3 -c '
import json, sys
eligible = {"api": [], "worker": []}
for pod in json.load(sys.stdin).get("items", []):
    metadata = pod.get("metadata", {})
    labels = metadata.get("labels", {})
    conditions = pod.get("status", {}).get("conditions", [])
    ready = any(condition.get("type") == "Ready" and condition.get("status") == "True" for condition in conditions)
    component = labels.get("app.kubernetes.io/component")
    if (component in eligible
            and pod.get("status", {}).get("phase") == "Running"
            and not metadata.get("deletionTimestamp") and ready):
        eligible[component].append(metadata["name"])
for component, names in eligible.items():
    if len(names) > 1:
        for name in names:
            print(name)
' <<<"$pod_list")"
  candidates=()
  if [[ -n "$candidate_list" ]]; then
    mapfile -t candidates <<<"$candidate_list"
  fi

  if (( ${#candidates[@]} == 0 )); then
    ((empty_polls += 1))
    if (( empty_polls >= 6 )); then
      echo "No redundant Ready API/worker pod available after 6 polls." >&2
      exit 1
    fi
    echo "Waiting for a redundant Ready API/worker pod." >&2
    sleep "$INTERVAL_SECONDS"
    continue
  fi

  empty_polls=0
  pod="$(printf '%s\n' "${candidates[@]}" | shuf -n 1)"
  echo "Deleting ${NAMESPACE}/${pod} ($((deleted + 1))/${MAX_DELETIONS})"
  kubectl --context "$KUBECTL_CONTEXT" -n "$NAMESPACE" delete pod "$pod" --wait=false
  ((deleted += 1))
  sleep "$INTERVAL_SECONDS"
done

echo "Chaos run completed: deleted ${deleted} API/worker pods."
