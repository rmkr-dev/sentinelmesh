#!/usr/bin/env python3
"""Check relative documentation links and balanced Mermaid fences."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LINK = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
SKIP = {".git", "bin", "dist"}


def main() -> int:
    errors: list[str] = []
    for path in ROOT.rglob("*.md"):
        if any(part in SKIP for part in path.parts):
            continue
        text = path.read_text(encoding="utf-8")
        if text.count("```mermaid") != text.count("```") - text.replace("```mermaid", "").count("```"):
            # Count mermaid fences by splitting. A mermaid block starts with ```mermaid and ends with ```.
            pass
        fences = re.findall(r"```mermaid[\s\S]*?```", text)
        if text.count("```mermaid") != len(fences):
            errors.append(f"{path.relative_to(ROOT)}: unclosed mermaid fence")
        for match in LINK.finditer(text):
            target = match.group(1).split("#", 1)[0].strip()
            if not target or target.startswith(("http://", "https://", "mailto:")):
                continue
            if target.startswith("/"):
                continue
            resolved = (path.parent / target).resolve()
            if not resolved.exists():
                errors.append(f"{path.relative_to(ROOT)}: missing {target}")
    if errors:
        print("\n".join(errors))
        return 1
    print("documentation links ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
