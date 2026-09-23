#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCAN_ROOTS = [ROOT / "backend", ROOT / "apps", ROOT / "packages"]
SOURCE_SUFFIXES = {".go", ".ts", ".tsx", ".js", ".jsx"}

FORBIDDEN_IMPORT_PATTERNS = [
    re.compile(r'["\'][^"\']*ClientPlatform[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*BusinessAIOS[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*Universal-Communication-Runtime[^"\']*["\']', re.I),
    re.compile(r'["\'][^"\']*Virtual-Persona-Runtime[^"\']*["\']', re.I),
]

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

if not (ROOT / "backend" / "go.mod").is_file():
    errors.append("backend/go.mod: canonical Go backend module is required")

for base in SCAN_ROOTS:
    if not base.exists():
        continue
    for path in base.rglob("*"):
        if not path.is_file() or path.suffix not in SOURCE_SUFFIXES:
            continue
        rel = path.relative_to(ROOT).as_posix()
        if "/connectors/" in f"/{rel}/" or "/adapters/" in f"/{rel}/":
            continue
        text = path.read_text(encoding="utf-8")
        for pattern in FORBIDDEN_IMPORT_PATTERNS:
            for match in pattern.finditer(text):
                line = text.count("\n", 0, match.start()) + 1
                errors.append(f"{rel}:{line}: provider/donor import outside connector boundary: {match.group(0)}")

if errors:
    print("ARCHITECTURE GUARD: FAIL")
    print("\n".join(errors))
    sys.exit(1)

print("ARCHITECTURE GUARD: PASS")
