from __future__ import annotations

from pathlib import Path
from typing import Any

REFERENCE_KEYS = ("implementation_refs", "contract_refs", "test_refs")
COMPLETION_STATUSES = {"VERIFIED", "RELEASED"}
COMPLETION_REFERENCE_KEYS = ("implementation_refs", "contract_refs", "test_refs", "evidence_refs")


def validate_completion_traceability(requirements: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    for requirement in requirements:
        status = requirement.get("status")
        if status not in COMPLETION_STATUSES:
            continue
        requirement_id = requirement.get("requirement_id") or "<unknown>"
        for key in COMPLETION_REFERENCE_KEYS:
            refs = requirement.get(key)
            if not isinstance(refs, list) or not refs:
                errors.append(f"{requirement_id}: {status} with empty {key}")
    return errors



def validate_repository_refs(
    root: Path,
    requirements: list[dict[str, Any]],
) -> list[str]:
    errors: list[str] = []
    root = root.resolve()

    for requirement in requirements:
        requirement_id = requirement.get("requirement_id") or "<unknown>"
        for key in REFERENCE_KEYS:
            refs = requirement.get(key) or []
            if not isinstance(refs, list):
                errors.append(f"{requirement_id}: {key} must be a list")
                continue

            for ref in refs:
                if not isinstance(ref, str) or not ref.strip():
                    errors.append(f"{requirement_id}: {key} contains an invalid ref")
                    continue

                relative = Path(ref.rstrip("/"))
                if relative.is_absolute() or ".." in relative.parts:
                    errors.append(
                        f"{requirement_id}: {key} ref escapes repository: {ref}"
                    )
                    continue

                candidate = (root / relative).resolve()
                try:
                    candidate.relative_to(root)
                except ValueError:
                    errors.append(
                        f"{requirement_id}: {key} ref escapes repository: {ref}"
                    )
                    continue

                if not candidate.exists():
                    errors.append(
                        f"{requirement_id}: {key} ref does not exist: {ref}"
                    )

    return errors
