#!/usr/bin/env python3
"""Bump the patch component of VERSION by 0.01 and sync the macOS plist."""

from __future__ import annotations

import plistlib
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
VERSION_FILE = ROOT / "VERSION"
PLIST_FILE = ROOT / "build" / "darwin" / "Info.plist"


def main() -> int:
    current = VERSION_FILE.read_text(encoding="utf-8").strip()
    parts = current.split(".")
    if not current or any(not part.isdigit() for part in parts):
        print(f"invalid VERSION: {current!r}", file=sys.stderr)
        return 1
    while len(parts) < 3:
        parts.append("0")
    parts = parts[:3]
    parts[2] = str(int(parts[2]) + 1)
    version = ".".join(parts)

    VERSION_FILE.write_text(version + "\n", encoding="utf-8")

    with PLIST_FILE.open("rb") as handle:
        plist = plistlib.load(handle)
    plist["CFBundleShortVersionString"] = version
    plist["CFBundleVersion"] = version
    with PLIST_FILE.open("wb") as handle:
        plistlib.dump(plist, handle)

    print(version)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
