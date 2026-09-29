#!/usr/bin/env python3
"""Fail when non-test sources contain unfinished or stand-in wording."""

import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
ALLOW = ROOT / "scripts" / "marker-allowlist.txt"
PATTERN = re.compile(
    r"\b(TODO|FIXME|XXX|HACK)\b|not implemented|panic\(\"unimplemented|\bstub\b|\bdummy\b|\bplaceholder\b|\bfake\b|\bmock\b",
    re.IGNORECASE,
)


def allowed(path: str, line: str) -> bool:
    if not ALLOW.exists():
        return False
    for raw in ALLOW.read_text().splitlines():
        raw = raw.strip()
        if not raw or raw.startswith("#"):
            continue
        prefix, _, snippet = raw.partition(":")
        if prefix == path and snippet in line:
            return True
    return False


def main() -> int:
    files = subprocess.check_output(
        ["git", "ls-files", "--", ":!*_test.go", ":!testdata/**", ":!CHANGELOG.md", ":!docs/development/**"],
        cwd=ROOT,
        text=True,
    ).splitlines()
    bad = []
    for rel in files:
        path = ROOT / rel
        if not path.is_file():
            continue
        try:
            text = path.read_text(errors="replace")
        except OSError:
            continue
        for n, line in enumerate(text.splitlines(), 1):
            if PATTERN.search(line) and not allowed(rel, line):
                bad.append(f"{rel}:{n}:{line.strip()}")
    if bad:
        print("\n".join(bad))
        return 1
    print(f"marker scan clean ({len(files)} files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
