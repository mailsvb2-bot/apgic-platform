from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tools.registry_ref_guard import validate_completion_traceability, validate_repository_refs


class RegistryRefGuardTests(unittest.TestCase):
    def test_accepts_existing_file_and_directory_refs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "backend/internal/example").mkdir(parents=True)
            (root / "backend/internal/example/domain.go").write_text(
                "package example\n",
                encoding="utf-8",
            )
            (root / "contracts").mkdir()
            (root / "contracts/example.json").write_text("{}\n", encoding="utf-8")
            (root / "tests").mkdir()
            (root / "tests/example_test.py").write_text("pass\n", encoding="utf-8")

            errors = validate_repository_refs(
                root,
                [{
                    "requirement_id": "APGIC-TEST-001",
                    "implementation_refs": ["backend/internal/example/"],
                    "contract_refs": ["contracts/example.json"],
                    "test_refs": ["tests/example_test.py"],
                }],
            )

            self.assertEqual(errors, [])

    def test_rejects_missing_refs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            errors = validate_repository_refs(
                Path(tmp),
                [{
                    "requirement_id": "APGIC-TEST-001",
                    "implementation_refs": ["missing/domain.go"],
                    "contract_refs": [],
                    "test_refs": [],
                }],
            )

            self.assertEqual(
                errors,
                [
                    "APGIC-TEST-001: implementation_refs ref does not exist: "
                    "missing/domain.go"
                ],
            )

    def test_rejects_absolute_and_parent_escape_refs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            errors = validate_repository_refs(
                Path(tmp),
                [{
                    "requirement_id": "APGIC-TEST-001",
                    "implementation_refs": ["/tmp/outside"],
                    "contract_refs": ["../outside"],
                    "test_refs": [],
                }],
            )

            self.assertEqual(len(errors), 2)
            self.assertTrue(all("escapes repository" in error for error in errors))

    def test_rejects_non_list_ref_blocks(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            errors = validate_repository_refs(
                Path(tmp),
                [{
                    "requirement_id": "APGIC-TEST-001",
                    "implementation_refs": "backend/internal/example",
                    "contract_refs": [],
                    "test_refs": [],
                }],
            )

            self.assertEqual(
                errors,
                ["APGIC-TEST-001: implementation_refs must be a list"],
            )


    def test_completion_requires_contract_and_all_traceability_refs(self) -> None:
        errors = validate_completion_traceability([{
            "requirement_id": "APGIC-EXEC-001",
            "status": "VERIFIED",
            "implementation_refs": ["tools/canon_lint.py"],
            "contract_refs": [],
            "test_refs": ["tools/tests/test_registry_ref_guard.py"],
            "evidence_refs": ["ci://candidate/example"],
        }])
        self.assertEqual(errors, ["APGIC-EXEC-001: VERIFIED with empty contract_refs"])

    def test_in_progress_may_keep_external_evidence_open(self) -> None:
        errors = validate_completion_traceability([{
            "requirement_id": "APGIC-EXEC-001",
            "status": "IN_PROGRESS",
            "implementation_refs": ["tools/canon_lint.py"],
            "contract_refs": [],
            "test_refs": ["tools/tests/test_registry_ref_guard.py"],
            "evidence_refs": [],
        }])
        self.assertEqual(errors, [])


if __name__ == "__main__":
    unittest.main()
