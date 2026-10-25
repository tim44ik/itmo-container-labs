#!/usr/bin/env python3
"""Run a sequential HTTP workload and save request-level evidence as CSV."""

import argparse
import collections
import csv
import datetime as dt
import http.client
import json
import statistics
import time
import urllib.parse
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--path", default="/orders")
    parser.add_argument("--method", default="GET")
    parser.add_argument("--requests", type=int, default=500)
    parser.add_argument("--output", required=True)
    parser.add_argument("--payload", default="")
    parser.add_argument("--interval-ms", type=int, default=0)
    args = parser.parse_args()
    if args.requests < 1 or args.interval_ms < 0:
        parser.error("requests must be positive and interval-ms must not be negative")
    endpoint = urllib.parse.urlsplit(args.url)
    if endpoint.scheme not in ("http", "https") or not endpoint.hostname:
        parser.error("url must be an absolute HTTP or HTTPS URL")
    port = endpoint.port or (443 if endpoint.scheme == "https" else 80)
    base_path = endpoint.path.rstrip("/")
    request_path = f"{base_path}{args.path}"
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    connection_type = (
        http.client.HTTPSConnection
        if endpoint.scheme == "https"
        else http.client.HTTPConnection
    )
    connection = connection_type(endpoint.hostname, port, timeout=10)
    elapsed_seconds = []
    statuses = collections.Counter()
    test_started = time.perf_counter()
    with output.open("w", newline="", encoding="utf-8") as csv_file:
        writer = csv.DictWriter(
            csv_file,
            fieldnames=("request", "timestamp_utc", "method", "path", "status", "latency_ms", "version"),
        )
        writer.writeheader()
        for request_number in range(1, args.requests + 1):
            headers = {"Content-Type": "application/json"} if args.payload else {}
            started = time.perf_counter()
            connection.request(args.method.upper(), request_path, body=args.payload or None, headers=headers)
            response = connection.getresponse()
            body = response.read()
            latency_ms = (time.perf_counter() - started) * 1000
            version = ""
            if request_path == "/version" and response.status == 200:
                try:
                    version = json.loads(body).get("version", "")
                except (json.JSONDecodeError, AttributeError):
                    version = body.decode("utf-8", errors="replace").strip()
            elapsed_seconds.append(latency_ms / 1000)
            statuses[str(response.status)] += 1
            writer.writerow(
                {
                    "request": request_number,
                    "timestamp_utc": dt.datetime.now(dt.timezone.utc).isoformat(),
                    "method": args.method.upper(),
                    "path": request_path,
                    "status": response.status,
                    "latency_ms": f"{latency_ms:.3f}",
                    "version": version,
                }
            )
            if args.interval_ms:
                time.sleep(args.interval_ms / 1000)
    connection.close()
    ordered = sorted(elapsed_seconds)
    summary = {
        "url": args.url,
        "path": request_path,
        "method": args.method.upper(),
        "requests": args.requests,
        "test_duration_seconds": time.perf_counter() - test_started,
        "total_request_latency_seconds": sum(elapsed_seconds),
        "status_counts": dict(statuses),
        "p50_ms": statistics.median(ordered) * 1000,
        "p95_ms": ordered[int(0.95 * (len(ordered) - 1))] * 1000,
        "mean_ms": statistics.mean(ordered) * 1000,
    }
    summary_path = output.with_suffix(".summary.json")
    summary_path.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
