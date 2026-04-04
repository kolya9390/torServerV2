#!/usr/bin/env python3
import argparse
import json
from pathlib import Path


LATENCY_UNITS = ["ns/op", "p50_ns/op", "p95_ns/op", "p99_ns/op"]
MEM_UNITS = ["B/op", "allocs/op"]


def load(path: str):
    return json.loads(Path(path).read_text())


def pct_change(old: float, new: float) -> float:
    if old == 0:
        return 0.0
    return (new - old) / old


def fmt_pct(v: float) -> str:
    return f"{v * 100:.2f}%"


def compare(base, latest, threshold):
    findings = []
    for bench, base_metrics in base.items():
        if bench not in latest:
            continue
        cur_metrics = latest[bench]
        for unit in LATENCY_UNITS + MEM_UNITS:
            if unit not in base_metrics or unit not in cur_metrics:
                continue
            old = float(base_metrics[unit]["mean"])
            new = float(cur_metrics[unit]["mean"])
            delta = pct_change(old, new)
            if delta > threshold:
                findings.append((bench, unit, old, new, delta))
    return findings


def build_report(base, latest):
    lines = [
        "# Benchmark Report",
        "",
        "| Benchmark | Unit | Baseline | Latest | Diff |",
        "|---|---:|---:|---:|---:|",
    ]
    for bench, base_metrics in sorted(base.items()):
        cur_metrics = latest.get(bench, {})
        for unit in ["p50_ns/op", "p95_ns/op", "p99_ns/op", "ns/op", "B/op", "allocs/op"]:
            if unit not in base_metrics or unit not in cur_metrics:
                continue
            old = float(base_metrics[unit]["mean"])
            new = float(cur_metrics[unit]["mean"])
            delta = pct_change(old, new)
            lines.append(f"| {bench} | {unit} | {old:.2f} | {new:.2f} | {fmt_pct(delta)} |")
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--baseline", required=True)
    ap.add_argument("--latest", required=True)
    ap.add_argument("--threshold", type=float, default=0.15)
    ap.add_argument("--report", required=True)
    args = ap.parse_args()

    baseline = load(args.baseline)
    latest = load(args.latest)
    findings = compare(baseline, latest, args.threshold)

    Path(args.report).write_text(build_report(baseline, latest))

    if findings:
        for bench, unit, old, new, delta in findings:
            print(
                f"[bench-regression] {bench} {unit}: baseline={old:.2f} latest={new:.2f} diff={fmt_pct(delta)}"
            )
        raise SystemExit(1)

    print("[bench-regression] no regressions above threshold")


if __name__ == "__main__":
    main()
