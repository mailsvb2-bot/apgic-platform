#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SURFACE_ROOTS = {
    "WEB": (ROOT / "apps/web",),
    "IOS": (ROOT / "apps/mobile",),
    "ANDROID": (ROOT / "apps/mobile",),
    "MOBILE": (ROOT / "apps/mobile",),
    "ALL": (ROOT / "apps/web", ROOT / "apps/mobile"),
}
CLIENT_ROOTS = SURFACE_ROOTS["ALL"]
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


def roots_for_surface(surface: str) -> tuple[Path, ...]:
    normalized = surface.strip().upper()
    try:
        return SURFACE_ROOTS[normalized]
    except KeyError as exc:
        raise ValueError(f"unsupported client surface: {surface}") from exc


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Reject service/server credential references from client source trees.")
    parser.add_argument(
        "--surface",
        choices=tuple(SURFACE_ROOTS),
        default="ALL",
        help="limit the scan to one client surface; IOS/ANDROID both scan the shared apps/mobile source tree",
    )
    args = parser.parse_args(argv)

    roots = roots_for_surface(args.surface)
    missing = [root for root in roots if not root.is_dir()]
    if missing:
        print("CLIENT CREDENTIAL GUARD: FAIL")
        for root in missing:
            print(f"ERROR: expected client root is missing: {root.as_posix()}")
        return 1

    errors = scan_client_roots(roots)
    if errors:
        print("CLIENT CREDENTIAL GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"CLIENT CREDENTIAL GUARD: PASS surface={args.surface}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
