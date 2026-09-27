#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SKIP_DIRS = {".git", "node_modules", "build", "dist", ".next", "Pods", "DerivedData"}
TEXT_SUFFIXES = {
    ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".json", ".yaml", ".yml",
    ".sql", ".md", ".sh", ".service", ".timer", ".xml", ".plist", ".gradle",
    ".properties", ".toml", ".txt", ".env", ".example", ".cjs", ".mjs",
}

PATTERNS = [
    ("private_key", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----")),
    ("github_token", re.compile(r"\bgh[pousr]_[A-Za-z0-9]{30,}\b")),
    ("aws_access_key", re.compile(r"\bAKIA[0-9A-Z]{16}\b")),
    ("stripe_live_secret", re.compile(r"\bsk_live_[0-9A-Za-z]{20,}\b")),
    ("openai_secret", re.compile(r"\bsk-[A-Za-z0-9_-]{32,}\b")),
    ("slack_token", re.compile(r"\bxox[baprs]-[0-9A-Za-z-]{20,}\b")),
    ("telegram_bot_token", re.compile(r"\b\d{7,12}:AA[A-Za-z0-9_-]{30,}\b")),
    (
        "basic_auth_url",
        re.compile(r"\b(?:https?|postgres(?:ql)?|redis)://[^\s/:@]+:[^\s/@]{8,}@[^\s]+", re.I),
    ),
]

PLACEHOLDER_FRAGMENTS = (
    "example", "placeholder", "change_me", "changeme", "dummy", "redacted",
    "<secret>", "***", "xxxx", "test-secret", "ci-secret",
)


def should_scan(path: Path) -> bool:
    if any(part in SKIP_DIRS for part in path.parts):
        return False
    if path.name.startswith(".env") and path.name != ".env.example":
        return True
    return path.suffix.lower() in TEXT_SUFFIXES or path.name in {
        "Dockerfile", "Makefile", "go.mod", "go.sum", "package.json", "package-lock.json"
    }


def is_placeholder(line: str) -> bool:
    lowered = line.lower()
    return any(fragment in lowered for fragment in PLACEHOLDER_FRAGMENTS)


def scan_repository(root: Path) -> list[str]:
    errors: list[str] = []
    for path in root.rglob("*"):
        rel_path = path.relative_to(root)
        if not path.is_file() or not should_scan(rel_path):
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except OSError as exc:
            errors.append(f"{rel_path.as_posix()}: could not read for secret scan: {exc}")
            continue
        for number, line in enumerate(text.splitlines(), start=1):
            if is_placeholder(line):
                continue
            for name, pattern in PATTERNS:
                if pattern.search(line):
                    errors.append(
                        f"{rel_path.as_posix()}:{number}: probable committed secret ({name})"
                    )
    return errors


def main() -> int:
    errors = scan_repository(ROOT)
    if errors:
        print("REPOSITORY SECRET SCAN: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("REPOSITORY SECRET SCAN: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
