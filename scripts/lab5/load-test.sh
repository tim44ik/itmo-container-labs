#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

URL="${URL:-http://127.0.0.1:18080/health}"
DURATION_SECONDS="${DURATION_SECONDS:-180}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-1}"
OUTPUT="${OUTPUT:-lab-5/evidence/load-test.csv}"

command -v curl >/dev/null || {
  echo "Required command not found: curl" >&2
  exit 1
}

mkdir -p "$(dirname "$OUTPUT")"
printf 'timestamp,success,http_code,time_seconds\n' >"$OUTPUT"
end_time=$((SECONDS + DURATION_SECONDS))

while (( SECONDS < end_time )); do
  timestamp="$(date --utc +'%Y-%m-%dT%H:%M:%SZ')"
  response="$(curl --silent --show-error --output /dev/null \
    --write-out '%{http_code},%{time_total}' --max-time 3 "$URL" 2>/dev/null)"
  curl_status=$?

  if (( curl_status == 0 )) && [[ "$response" == 2* ]]; then
    success=true
  else
    success=false
    response="${response:-000,0}"
  fi

  printf '%s,%s,%s\n' "$timestamp" "$success" "$response" | tee -a "$OUTPUT"
  sleep "$INTERVAL_SECONDS"
done

echo "Load results saved to $OUTPUT"
