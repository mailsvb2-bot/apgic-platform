#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLIENT_ROOTS = (ROOT / "apps/web", ROOT / "apps/native")
TEXT_SUFFIXES = {".ts", ".tsx", ".js", ".jsx", ".json", ".yaml", ".yml", ".plist", ".xml", ".gradle", ".properties"}

PATTERNS = (
    re.compile(r"\b(?:NEXT_PUBLIC|EXPO_PUBLIC)_[A-Z0-9_]*(?:SECRET|PRIVATE_KEY|SERVICE_TOKEN|CLIENT_SECRET)\b"),
    re.compile(r"\b(?:SERVICE_PRINCIPAL_SECRET|CONNECTOR_CLIENT_SECRET|PROVIDER_CLIENT_SECRET|PAYMENT_PROVIDER_SECRET)\b"),
    re.compile(r"process\.env\.[A-Z0-9_]*(?:SECRET|PRIVATE_KEY|SERVICE_TOKEN|CLIENT_SECRET)\b"),
)


def scan_file(path: Path) -> list[str]:
    text = path.read_text(encoding="utf-8", errors="ignore")
    errors: list[str] = []
    for number, line in enumerate(text.splitlines(), start=1):
        if any(pattern.search(line) for pattern in PATTERNS):
            errors.append(f"{path.as_posix()}:{number}: client credential material/reference is forbidden")
    return errors


def scan_client_roots(roots: tuple[Path, ...] = CLIENT_ROOTS) -> list[str]:
    errors: list[str] = []
    for root in roots:
        if not root.exists():
            continue
        for path in root.rglob("*"):
            if path.is_file() and path.suffix.lower() in TEXT_SUFFIXES:
                errors.extend(scan_file(path))
    return errors


def main() -> int:
    errors = scan_client_roots()
    if errors:
        print("CLIENT CREDENTIAL GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("CLIENT CREDENTIAL GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
