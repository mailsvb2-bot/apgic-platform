#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
TERMS = ROOT / "canon/contracts/provider-terminology.yaml"
CONNECTOR_REGISTRY = ROOT / "backend/internal/connector/registry.go"

errors: list[str] = []
doc = yaml.safe_load(TERMS.read_text(encoding="utf-8")) or {}
expected_capabilities = set(doc.get("canonical_capability_classes") or [])
forbidden_fragments = [
    str(item).lower()
    for item in (doc.get("forbidden_canonical_brand_fragments") or [])
]

if not expected_capabilities:
    errors.append("canonical capability class registry is empty")

registry_source = CONNECTOR_REGISTRY.read_text(encoding="utf-8")
actual_capabilities = set(
    re.findall(r'Capability[A-Za-z0-9_]+\s+CapabilityClass\s*=\s*"([A-Z0-9_]+)"', registry_source)
)

missing = sorted(expected_capabilities - actual_capabilities)
extra = sorted(actual_capabilities - expected_capabilities)
if missing:
    errors.append(f"connector registry missing canonical capabilities: {missing}")
if extra:
    errors.append(f"connector registry has unregistered canonical capabilities: {extra}")

identifier_patterns = (
    re.compile(r"\btype\s+([A-Za-z_][A-Za-z0-9_]*)\b"),
    re.compile(r"\b(?:interface|class)\s+([A-Za-z_][A-Za-z0-9_]*)\b"),
    re.compile(r"\bexport\s+type\s+([A-Za-z_][A-Za-z0-9_]*)\b"),
)

scan_roots = [
    ROOT / "backend/internal",
    ROOT / "packages/contracts/src",
]
for base in scan_roots:
    for path in base.rglob("*"):
        if not path.is_file() or path.suffix.lower() not in {".go", ".ts", ".tsx"}:
            continue
        rel = path.relative_to(ROOT).as_posix()
        if "/connector/" in f"/{rel}/" or "/connectors/" in f"/{rel}/" or "/adapters/" in f"/{rel}/":
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        identifiers: set[str] = set()
        for pattern in identifier_patterns:
            identifiers.update(pattern.findall(text))
        for identifier in identifiers:
            lowered = identifier.lower()
            for fragment in forbidden_fragments:
                if fragment in lowered:
                    errors.append(
                        f"{rel}: canonical identifier {identifier!r} is brand/donor-coupled"
                    )

for schema_path in (ROOT / "contracts").rglob("*"):
    if not schema_path.is_file() or schema_path.suffix.lower() not in {".yaml", ".yml", ".json"}:
        continue
    rel = schema_path.relative_to(ROOT).as_posix()
    text = schema_path.read_text(encoding="utf-8", errors="ignore").lower()
    for fragment in forbidden_fragments:
        if fragment in text:
            errors.append(f"{rel}: canonical contract contains brand/donor fragment {fragment}")

if re.search(r"\bpayment_type\b", registry_source, re.I):
    errors.append("connector registry must not collapse payment provider/method/rail into payment_type")

if errors:
    print("TERMINOLOGY GUARD: FAIL")
    for error in errors:
        print(f"ERROR: {error}")
    sys.exit(1)

print(
    "TERMINOLOGY GUARD: PASS "
    f"(capabilities={len(expected_capabilities)}, forbidden_fragments={len(forbidden_fragments)})"
)
