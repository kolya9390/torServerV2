#!/usr/bin/env python3
import argparse
import json
import re
import statistics
from pathlib import Path


BENCH_RE = re.compile(r"^(Benchmark\S+)-\d+\s+\d+\s+(.*)$")
PAIR_RE = re.compile(r"([0-9]+(?:\.[0-9]+)?)\s+([A-Za-z0-9_./%-]+)")


def parse_lines(text: str):
    buckets = {}
    for line in text.splitlines():
        m = BENCH_RE.match(line.strip())
        if not m:
            continue
        name = m.group(1)
        rest = m.group(2)
        pairs = PAIR_RE.findall(rest)
        if not pairs:
            continue
        metrics = buckets.setdefault(name, {})
        for value, unit in pairs:
            metrics.setdefault(unit, []).append(float(value))
    return buckets


def collapse(buckets):
    out = {}
    for bench, metrics in buckets.items():
        out[bench] = {}
        for unit, values in metrics.items():
            out[bench][unit] = {
                "samples": len(values),
                "mean": statistics.mean(values),
                "min": min(values),
                "max": max(values),
            }
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    text = Path(args.input).read_text()
    buckets = parse_lines(text)
    data = collapse(buckets)
    Path(args.output).write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
