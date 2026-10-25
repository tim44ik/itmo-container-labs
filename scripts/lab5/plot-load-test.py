#!/usr/bin/env python3
import csv
import html
import sys
from datetime import datetime
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: plot-load-test.py INPUT.csv OUTPUT.svg", file=sys.stderr)
        return 2

    source = Path(sys.argv[1])
    target = Path(sys.argv[2])
    rows = []
    with source.open(newline="", encoding="utf-8") as handle:
        for row in csv.DictReader(handle):
            rows.append(
                (
                    datetime.fromisoformat(row["timestamp"].replace("Z", "+00:00")),
                    row["success"].lower() == "true",
                    float(row["time_seconds"]),
                )
            )

    if not rows:
        print(f"No measurements in {source}", file=sys.stderr)
        return 1

    failures = sum(not success for _, success, _ in rows)
    width, height = 1100, 360
    left, right, top, bottom = 70, 20, 48, 50
    plot_width, plot_height = width - left - right, height - top - bottom
    max_latency = max(max(latency for _, _, latency in rows), 0.1)

    def x_position(index: int) -> float:
        return left + (plot_width * index / max(len(rows) - 1, 1))

    def y_position(latency: float) -> float:
        return top + plot_height * (1 - min(latency, max_latency) / max_latency)

    points = []
    for index, (_, success, latency) in enumerate(rows):
        color = "#27864b" if success else "#c0392b"
        points.append(
            f'<circle cx="{x_position(index):.1f}" cy="{y_position(latency):.1f}" '
            f'r="3" fill="{color}"/>'
        )

    first = html.escape(rows[0][0].strftime("%H:%M:%S UTC"))
    last = html.escape(rows[-1][0].strftime("%H:%M:%S UTC"))
    outcome = "PASS: zero failed requests" if failures == 0 else f"FAIL: {failures} failed requests"
    svg = f"""<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">
<rect width="100%" height="100%" fill="white"/>
<text x="{left}" y="25" font-family="sans-serif" font-size="18">NodePort health requests during pod chaos</text>
<text x="{left}" y="43" font-family="sans-serif" font-size="13">{html.escape(outcome)} — {len(rows)} requests</text>
<line x1="{left}" y1="{top}" x2="{left}" y2="{top + plot_height}" stroke="#777"/>
<line x1="{left}" y1="{top + plot_height}" x2="{left + plot_width}" y2="{top + plot_height}" stroke="#777"/>
<text x="12" y="{top + plot_height / 2}" transform="rotate(-90 12 {top + plot_height / 2})" font-family="sans-serif" font-size="12">response seconds</text>
<text x="{left}" y="{height - 12}" font-family="sans-serif" font-size="12">{first}</text>
<text x="{width - right - 100}" y="{height - 12}" font-family="sans-serif" font-size="12">{last}</text>
{''.join(points)}
</svg>
"""
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(svg, encoding="utf-8")
    print(f"{outcome}; graph written to {target}")
    return 0 if failures == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
