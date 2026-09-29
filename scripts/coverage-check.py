#!/usr/bin/env python3
"""Fail when statement coverage is below the package targets."""

import sys
from collections import defaultdict
from pathlib import Path

TARGETS = {
    "internal/api": 75,
    "internal/shop": 60,
    "internal/auth": 80,
    "internal/telemetry": 60,
    "internal/azure/auth": 75,
    "internal/telemetryquery": 70,
    "internal/config": 80,
    "internal/faults": 70,
    "cmd/obsctl": 50,
    "cmd/platform": 50,
}
TOTAL = 70


def package_of(filename: str) -> str:
    parts = filename.split("/")
    if filename.startswith("github.com/"):
        parts = parts[3:]
    if parts[0] == "cmd" and len(parts) > 1:
        return "/".join(parts[:2])
    if len(parts) > 2 and parts[0] == "internal" and parts[1] == "azure" and parts[2] == "auth":
        return "internal/azure/auth"
    if parts[0] == "internal" and len(parts) > 1:
        return "/".join(parts[:2])
    return parts[0]


def main() -> int:
    profile = Path(sys.argv[1] if len(sys.argv) > 1 else "cover.out")
    covered = defaultdict(int)
    total = defaultdict(int)
    for line in profile.read_text().splitlines()[1:]:
        bits = line.split()
        if len(bits) != 3:
            continue
        meta, nstmt_s, hits_s = bits
        nstmt = int(nstmt_s)
        hits = int(hits_s)
        file_name = meta.split(":")[0]
        pkg = package_of(file_name)
        total[pkg] += nstmt
        total["__total"] += nstmt
        if hits > 0:
            covered[pkg] += nstmt
            covered["__total"] += nstmt
    failed = False
    grand = 100 * covered["__total"] / total["__total"] if total["__total"] else 0
    print(f"total {grand:.1f}% (target {TOTAL}%)")
    if grand + 1e-9 < TOTAL:
        failed = True
    for pkg, target in TARGETS.items():
        if total[pkg] == 0:
            print(f"{pkg}: no statements")
            failed = True
            continue
        pct = 100 * covered[pkg] / total[pkg]
        print(f"{pkg} {pct:.1f}% (target {target}%)")
        if pct + 1e-9 < target:
            failed = True
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
