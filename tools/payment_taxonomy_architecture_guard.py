#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCAN_ROOTS = [ROOT / "backend", ROOT / "apps", ROOT / "packages", ROOT / "contracts"]
SOURCE_SUFFIXES = {".go", ".ts", ".tsx", ".js", ".jsx", ".sql", ".json", ".yaml", ".yml"}
MIXED_PAYMENT_TYPE = re.compile(r"\bpayment_type\b", re.I)

REQUIRED_ANCHORS = {
    "backend/internal/payments/taxonomy.go": [
        r"type\s+ProviderID\s+string",
        r"type\s+MethodCode\s+string",
        r"type\s+RailCode\s+string",
        r'ProviderID\s+ProviderID\s+`json:"provider_id"`',
        r'MethodCode\s+MethodCode\s+`json:"method_code"`',
        r'RailCode\s+RailCode\s+`json:"rail_code"`',
    ],
    "contracts/openapi/apgic-v1.yaml": [
        r"required:\s*\[provider_id, method_code, rail_code\]",
        r"(?m)^\s+provider_id:\s*$",
        r"(?m)^\s+method_code:\s*$",
        r"(?m)^\s+rail_code:\s*$",
    ],
    "backend/migrations/000009_r2_payment_control_plane.sql": [
        r"method_codes\s+text\[\]",
        r"rail_codes\s+text\[\]",
        r"selected_method_code\s+text",
        r"selected_rail_code\s+text",
        r"provider_config_id\s+uuid",
    ],
}


def is_test_source(rel: str) -> bool:
    return rel.endswith(("_test.go", ".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx"))


def validate_payment_taxonomy(root: Path) -> list[str]:
    errors: list[str] = []

    for base in [root / item.relative_to(ROOT) for item in SCAN_ROOTS]:
        if not base.exists():
            continue
        for path in base.rglob("*"):
            if not path.is_file() or path.suffix.lower() not in SOURCE_SUFFIXES:
                continue
            rel = path.relative_to(root).as_posix()
            if is_test_source(rel):
                continue
            text = path.read_text(encoding="utf-8", errors="ignore")
            match = MIXED_PAYMENT_TYPE.search(text)
            if match:
                line = text.count("\n", 0, match.start()) + 1
                errors.append(
                    f"{rel}:{line}: mixed payment_type semantics are forbidden; "
                    "keep provider, method and rail separate"
                )

    for rel, patterns in REQUIRED_ANCHORS.items():
        path = root / rel
        if not path.is_file():
            errors.append(f"{rel}: required payment taxonomy architecture source missing")
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        for pattern in patterns:
            if not re.search(pattern, text):
                errors.append(f"{rel}: missing payment taxonomy architecture anchor {pattern}")

    return errors


def main() -> int:
    errors = validate_payment_taxonomy(ROOT)
    if errors:
        print("PAYMENT TAXONOMY ARCHITECTURE GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("PAYMENT TAXONOMY ARCHITECTURE GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
