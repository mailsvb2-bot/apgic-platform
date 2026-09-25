#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCAN_ROOTS = [
    ROOT / "backend" / "migrations",
    ROOT / "backend" / "internal" / "payments",
    ROOT / "contracts",
]

STRUCTURAL_PATTERNS = [
    (
        re.compile(
            r"\bCREATE\s+TABLE\s+(?:[a-zA-Z_][\w]*\.)?"
            r"(?:wallets?|balances?|custodial_accounts?|stored_value_accounts?)\b",
            re.I,
        ),
        "APGIC-owned monetary wallet/balance table is forbidden",
    ),
    (
        re.compile(
            r"(?mi)^\s*(?:apgic_|cash_|wallet_|custodial_|stored_value_)?"
            r"balance(?:_minor)?\s+(?:bigint|integer|numeric|decimal|money)\b"
        ),
        "custodial monetary balance column is forbidden",
    ),
    (
        re.compile(r"(?mi)^\s*(?:card_number|pan|cvv|cvc|track_data)\s+"),
        "raw card/payment credential storage is forbidden",
    ),
    (
        re.compile(
            r"(?m)^\s*type\s+(?:Wallet|CustodialAccount|StoredValueAccount|"
            r"MonetaryBalance)\b"
        ),
        "APGIC custodial money domain type is forbidden",
    ),
]

errors: list[str] = []

for base in SCAN_ROOTS:
    if not base.exists():
        continue
    for path in base.rglob("*"):
        if not path.is_file() or path.suffix.lower() not in {
            ".go",
            ".sql",
            ".json",
            ".yaml",
            ".yml",
            ".ts",
            ".tsx",
        }:
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        rel = path.relative_to(ROOT).as_posix()
        for pattern, message in STRUCTURAL_PATTERNS:
            for match in pattern.finditer(text):
                line = text.count("\n", 0, match.start()) + 1
                errors.append(f"{rel}:{line}: {message}")

if errors:
    print("PAYMENT CUSTODY GUARD: FAIL")
    for error in errors:
        print(f"ERROR: {error}")
    sys.exit(1)

print("PAYMENT CUSTODY GUARD: PASS")
