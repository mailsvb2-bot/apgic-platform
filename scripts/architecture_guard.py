#!/usr/bin/env python3
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
SCAN_ROOTS = [
    ROOT / "cmd",
    ROOT / "internal",
    ROOT / "migrations",
    ROOT / "apps",
    ROOT / "packages",
]

FORBIDDEN_DONOR_RUNTIME = (
    "businesaios",
    "clientplatform",
    "universal-communication-runtime",
    "virtual-persona-runtime",
)

errors: list[str] = []

for root in SCAN_ROOTS:
    if not root.exists():
        continue
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if path.suffix.lower() not in {".go", ".sql", ".ts", ".tsx", ".js", ".json", ".md"}:
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        lowered = text.lower()
        for donor in FORBIDDEN_DONOR_RUNTIME:
            if donor in lowered:
                errors.append(f"{path.relative_to(ROOT)} imports/references donor runtime: {donor}")
        if re.search(r"\bpayment_type\b", text):
            errors.append(f"{path.relative_to(ROOT)} collapses provider/method/rail into payment_type")
        if path.parts and ("apps" in path.parts or "packages" in path.parts):
            if re.search(r"(?:from|require\s*\()\s*['\"](?:\.\./)*internal/", text):
                errors.append(f"{path.relative_to(ROOT)} imports server business logic")

for path in ROOT.rglob(".env*"):
    if path.name not in {".env.example"}:
        errors.append(f"secret-bearing env file must not be committed: {path.relative_to(ROOT)}")

required_paths = [
    ROOT / "apps" / "web",
    ROOT / "apps" / "native",
    ROOT / "packages" / "contracts",
]
for path in required_paths:
    if not path.exists():
        errors.append(f"missing multi-surface boundary: {path.relative_to(ROOT)}")

if errors:
    for error in errors:
        print(f"ARCH_GUARD_FAIL: {error}", file=sys.stderr)
    raise SystemExit(1)

print("ARCH_GUARD_OK")
