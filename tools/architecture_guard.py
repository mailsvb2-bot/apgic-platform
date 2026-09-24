#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCAN_ROOTS = [ROOT / "backend", ROOT / "apps", ROOT / "packages", ROOT / "contracts"]
SOURCE_SUFFIXES = {".go", ".ts", ".tsx", ".js", ".jsx", ".sql", ".json", ".yaml", ".yml"}

FORBIDDEN_IMPORT_PATTERNS = [
    re.compile(r'["\'][^"\']*ClientPlatform[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*BusinessAIOS[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*Universal-Communication-Runtime[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*Virtual-Persona-Runtime[^"\']*["\']', re.I),
]
PAYMENT_TYPE = re.compile(r"\bpayment_type\b", re.I)
CUSTODIAL_MONEY_TYPE = re.compile(\n    r"\\b(?:CustodialBalance|MonetaryWallet|StoredValueAccount|UserMoneyBalance|SpecialistMoneyBalance|PayoutBalance)\\b"\n)\nCLIENT_SERVER_IMPORT = re.compile(r"(?:backend[/\\]internal|from\s+['\"][^'\"]*backend)", re.I)

FORBIDDEN_DUPLICATE_ROOTS = [
    ROOT / "go.mod",
    ROOT / "cmd",
    ROOT / "internal",
    ROOT / "migrations",
    ROOT / "scripts",
    ROOT / "apps" / "native",
    ROOT / "canon" / "APGIC_Requirement_Registry_v7_FINAL.yaml",
    ROOT / "canon" / "APGIC_Canon_Coverage_v7_FINAL.json",
]

errors: list[str] = []

for duplicate in FORBIDDEN_DUPLICATE_ROOTS:
    if duplicate.exists():
        errors.append(
            f"{duplicate.relative_to(ROOT).as_posix()}: duplicate canonical ownership; "
            "server truth belongs under backend/, native under apps/mobile/, "
            "tooling under tools/, and executable canon under canon/requirements/"
        )

required_paths = [
    ROOT / "backend" / "go.mod",
    ROOT / "apps" / "web" / "package.json",
    ROOT / "apps" / "mobile" / "package.json",
    ROOT / "packages" / "contracts" / "src" / "index.ts",
    ROOT / "canon" / "requirements" / "registry.yaml",
    ROOT / "canon" / "requirements" / "coverage.json",
]
for required in required_paths:
    if not required.is_file():
        errors.append(f"{required.relative_to(ROOT).as_posix()}: required canonical path missing")

for base in SCAN_ROOTS:
    if not base.exists():
        continue
    for path in base.rglob("*"):
        if not path.is_file() or path.suffix.lower() not in SOURCE_SUFFIXES:
            continue
        rel = path.relative_to(ROOT).as_posix()
        text = path.read_text(encoding="utf-8", errors="ignore")

        is_test = rel.endswith(("_test.go", ".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx"))

        if PAYMENT_TYPE.search(text) and not is_test:
            errors.append(f"{rel}: mixed payment_type semantics are forbidden; use provider/method/rail")

        if CUSTODIAL_MONEY_TYPE.search(text) and not is_test:
            errors.append(f"{rel}: APGIC custodial/stored-value money types are forbidden; monetary execution belongs to external providers")

        if rel.startswith(("apps/", "packages/")) and CLIENT_SERVER_IMPORT.search(text):
            errors.append(f"{rel}: client/shared package imports server business logic")

        if "/connectors/" in f"/{rel}/" or "/connector/" in f"/{rel}/" or "/adapters/" in f"/{rel}/":
            continue
        for pattern in FORBIDDEN_IMPORT_PATTERNS:
            for match in pattern.finditer(text):
                line = text.count("\n", 0, match.start()) + 1
                errors.append(f"{rel}:{line}: provider/donor import outside connector boundary: {match.group(0)}")

for path in ROOT.rglob(".env*"):
    if path.name != ".env.example":
        errors.append(f"{path.relative_to(ROOT).as_posix()}: secret-bearing env file must not be committed")

if errors:
    print("ARCHITECTURE GUARD: FAIL")
    print("\n".join(f"ERROR: {item}" for item in errors))
    sys.exit(1)

print("ARCHITECTURE GUARD: PASS")
