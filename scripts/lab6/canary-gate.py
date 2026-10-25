#!/usr/bin/env python3
"""Promote a canary when Prometheus metrics are healthy; roll it back otherwise."""

import json
import logging
import os
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


LOG = logging.getLogger("canary-gate")


def env_float(name, default):
    try:
        value = float(os.environ.get(name, default))
    except ValueError as error:
        raise ValueError(f"{name} must be numeric") from error
    if value < 0:
        raise ValueError(f"{name} must be non-negative")
    return value


def env_int(name, default):
    try:
        value = int(os.environ.get(name, default))
    except ValueError as error:
        raise ValueError(f"{name} must be an integer") from error
    if value < 1:
        raise ValueError(f"{name} must be at least 1")
    return value


PROMETHEUS_URL = os.environ.get("PROMETHEUS_URL", "http://127.0.0.1:19090").rstrip("/")
NAMESPACE = os.environ.get("NAMESPACE", "shop")
RELEASE = os.environ.get("RELEASE", "shop")
INTERVAL_SECONDS = env_int("INTERVAL_SECONDS", "15")
WINDOW = os.environ.get("PROMETHEUS_WINDOW", "2m")
ERROR_THRESHOLD = env_float("MAX_ERROR_RATIO", "0.05")
P95_THRESHOLD = env_float("MAX_P95_SECONDS", "0.2")
MIN_REQUESTS = env_int("MIN_REQUESTS", "20")
STEP_PERCENT = env_int("PROMOTION_STEP_PERCENT", "10")
HEALTHY_WINDOWS = env_int("HEALTHY_WINDOWS_REQUIRED", "2")
MAX_ITERATIONS = int(os.environ.get("MAX_ITERATIONS", "0"))
KUBE_CONTEXT = os.environ.get("KUBE_CONTEXT", "")
V1_SERVICE = f"{RELEASE}-api-v1"
V2_SERVICE = f"{RELEASE}-api-v2"
ROUTES = (f"{RELEASE}-api", f"{RELEASE}-api-retries")


def prom_scalar(expression):
    query = urllib.parse.urlencode({"query": expression})
    request = urllib.request.Request(
        f"{PROMETHEUS_URL}/api/v1/query?{query}",
        headers={"Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            payload = json.load(response)
    except urllib.error.HTTPError as error:
        details = error.read(1024).decode("utf-8", errors="replace")
        raise RuntimeError(f"Prometheus returned HTTP {error.code}: {details}") from error
    except urllib.error.URLError as error:
        raise RuntimeError(f"Prometheus query failed: {error.reason}") from error
    if payload.get("status") != "success":
        raise RuntimeError(f"Prometheus query was not successful: {payload}")
    results = payload.get("data", {}).get("result", [])
    if not results:
        return None
    return float(results[0]["value"][1])


def query_canary_metrics():
    selector = f'namespace="{NAMESPACE}",service="{V2_SERVICE}"'
    total = f"sum(increase(http_requests_total{{{selector}}}[{WINDOW}]))"
    errors = (
        f'sum(increase(http_requests_total{{{selector},status=~"5.."}}[{WINDOW}]))'
    )
    p95 = (
        "histogram_quantile(0.95, "
        f'sum by (le) (rate(http_request_duration_seconds_bucket{{{selector}}}[{WINDOW}])))'
    )
    total_requests = prom_scalar(total)
    if total_requests is None:
        return None
    error_count = prom_scalar(errors)
    latency = prom_scalar(p95)
    if error_count is None or latency is None:
        return None
    return {
        "requests": total_requests,
        "error_ratio": error_count / max(total_requests, 1.0),
        "p95_seconds": latency,
    }


def kubectl(*args):
    command = ["kubectl"]
    if KUBE_CONTEXT:
        command.extend(["--context", KUBE_CONTEXT])
    command.extend(args)
    return subprocess.run(command, check=True, capture_output=True, text=True)


def route_weights(route_name):
    result = kubectl(
        "-n",
        NAMESPACE,
        "get",
        "httproute",
        route_name,
        "-o",
        "json",
    )
    route = json.loads(result.stdout)
    weights = {}
    for rule_index, rule in enumerate(route.get("spec", {}).get("rules", [])):
        for backend_index, backend in enumerate(rule.get("backendRefs", [])):
            name = backend.get("name")
            if name in (V1_SERVICE, V2_SERVICE):
                weights[name] = (
                    rule_index,
                    backend_index,
                    int(backend.get("weight", 1)),
                )
    if V1_SERVICE not in weights or V2_SERVICE not in weights:
        raise RuntimeError(f"HTTPRoute {route_name} does not reference both API versions")
    return weights


def set_canary_weight(v2_weight):
    v2_weight = max(0, min(100, int(v2_weight)))
    patch = []
    for route_name in ROUTES:
        weights = route_weights(route_name)
        v1_location = weights[V1_SERVICE]
        v2_location = weights[V2_SERVICE]
        operations = [
            {
                "op": "replace",
                "path": f"/spec/rules/{v1_location[0]}/backendRefs/{v1_location[1]}/weight",
                "value": 100 - v2_weight,
            },
            {
                "op": "replace",
                "path": f"/spec/rules/{v2_location[0]}/backendRefs/{v2_location[1]}/weight",
                "value": v2_weight,
            },
        ]
        kubectl(
            "-n",
            NAMESPACE,
            "patch",
            "httproute",
            route_name,
            "--type=json",
            "-p",
            json.dumps(operations),
        )
        patch.append(route_name)
    LOG.info(
        "updated HTTPRoute weights",
        extra={"routes": patch, "v1_weight": 100 - v2_weight, "v2_weight": v2_weight},
    )


def decide(metrics, current_weight, good_windows):
    if metrics is None or metrics["requests"] < MIN_REQUESTS:
        return current_weight, 0, "insufficient_sample"
    if metrics["error_ratio"] > ERROR_THRESHOLD or metrics["p95_seconds"] > P95_THRESHOLD:
        return 0, 0, "rollback_threshold_exceeded"
    good_windows += 1
    if good_windows < HEALTHY_WINDOWS or current_weight == 100:
        return current_weight, good_windows, "healthy_window"
    return min(100, current_weight + STEP_PERCENT), 0, "promote"


def log_decision(metrics, current_weight, next_weight, reason):
    LOG.info(
        json.dumps(
            {
                "event": "canary_decision",
                "reason": reason,
                "v2_weight_before": current_weight,
                "v2_weight_after": next_weight,
                "metrics": metrics,
            },
            sort_keys=True,
        )
    )


def main():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )
    if STEP_PERCENT > 100 or ERROR_THRESHOLD > 1:
        raise ValueError("promotion step must be <= 100 and error threshold must be <= 1")
    current = route_weights(ROUTES[0])[V2_SERVICE][2]
    good_windows = 0
    iteration = 0
    LOG.info(
        "gate started namespace=%s v2_weight=%d interval=%ss",
        NAMESPACE,
        current,
        INTERVAL_SECONDS,
    )
    while MAX_ITERATIONS == 0 or iteration < MAX_ITERATIONS:
        iteration += 1
        try:
            metrics = query_canary_metrics()
        except RuntimeError as error:
            LOG.error("%s; no traffic weights changed", error)
            good_windows = 0
        else:
            next_weight, good_windows, reason = decide(metrics, current, good_windows)
            log_decision(metrics, current, next_weight, reason)
            if next_weight != current:
                set_canary_weight(next_weight)
                current = next_weight
        if MAX_ITERATIONS and iteration >= MAX_ITERATIONS:
            break
        time.sleep(INTERVAL_SECONDS)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        LOG.error("%s", error)
        sys.exit(1)
