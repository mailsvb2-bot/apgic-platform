from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "canon_execution_gap_inventory", ROOT / "tools/canon_execution_gap_inventory.py"
)
assert SPEC is not None and SPEC.loader is not None
inventory_tool = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(inventory_tool)


class CanonGapInventoryTests(unittest.TestCase):
    @staticmethod
    def fixtures():
        coverage = {
            "meta": {"version": "v7_FINAL"},
            "sections": [
                {"section": 1, "title": "Mission", "owner_domain": "DEMAND",
                 "coverage_status": "SECTION_OWNER_BASELINE", "linked_requirement_ids": []},
                {"section": 2, "title": "Booking", "owner_domain": "BOOKING",
                 "coverage_status": "EXPLICIT_REQUIREMENT_LINK", "linked_requirement_ids": ["APGIC-BOOK-001"]},
                {"section": 3, "title": "Payment", "owner_domain": "PAY",
                 "coverage_status": "EXPLICIT_REQUIREMENT_LINK", "linked_requirement_ids": ["APGIC-PAY-001"]},
            ],
        }
        registry = {"requirements": [
            {"requirement_id": "APGIC-PAY-001", "title": "Payment", "owner_domain": "PAY",
             "source_sections": [3], "release_profile": "R2", "priority": "P0", "status": "VERIFIED",
             "dependencies": ["APGIC-BOOK-001"], "implementation_refs": ["backend/payment.go"],
             "test_refs": [], "evidence_refs": []},
            {"requirement_id": "APGIC-BOOK-001", "title": "Booking", "owner_domain": "BOOKING",
             "source_sections": [2], "release_profile": "R2", "priority": "P0", "status": "IN_PROGRESS",
             "dependencies": [], "implementation_refs": [], "test_refs": [], "evidence_refs": []},
        ]}
        return coverage, registry, {"records": {}}

    def test_owner_only_not_mislabeled_as_unimplemented(self):
        coverage, registry, production = self.fixtures()
        result = inventory_tool.build_inventory(coverage, registry, production)
        self.assertEqual(result["totals"]["canon_sections"], 3)
        self.assertEqual(result["totals"]["owner_only_sections_needing_analysis"], 1)
        self.assertEqual(result["totals"]["requirements"], 2)
        self.assertEqual(result["totals"]["non_released_requirements"], 2)
        self.assertEqual(len(result["all_requirements_dependency_order"]), 2)
        self.assertEqual(result["owner_only_sections"][0]["tracking_class"], "OWNER_ONLY_REQUIRES_ANALYSIS")
        self.assertEqual(result["requirements_backlog_dependency_order"][0]["requirement_id"], "APGIC-BOOK-001")
        self.assertEqual(result["requirements_backlog_dependency_order"][1]["unverified_dependencies"], ["APGIC-BOOK-001"])
        self.assertEqual(result["totals"]["traceability_mismatches"], 0)

    def test_freeze_disagreement_is_visible_not_silently_corrected(self):
        coverage, registry, production = self.fixtures()
        coverage["sections"][1]["linked_requirement_ids"] = []
        result = inventory_tool.build_inventory(coverage, registry, production)
        self.assertEqual(result["totals"]["traceability_mismatches"], 1)
        self.assertEqual(result["traceability_mismatches"][0]["registry_requirement_ids"], ["APGIC-BOOK-001"])
        self.assertEqual(registry["requirements"][1]["source_sections"], [2])

    def test_released_requirements_stay_in_full_index(self):
        coverage, registry, production = self.fixtures()
        registry["requirements"][0]["status"] = "RELEASED"
        result = inventory_tool.build_inventory(coverage, registry, production)
        self.assertEqual(len(result["all_requirements_dependency_order"]), 2)
        self.assertEqual(len(result["requirements_backlog_dependency_order"]), 1)
        released = result["all_requirements_dependency_order"][-1]
        self.assertEqual(released["requirement_id"], "APGIC-PAY-001")
        self.assertEqual(released["status"], "RELEASED")
        self.assertEqual(released["implementation_refs"], ["backend/payment.go"])

    def test_strict_final_refuses_forged_production_evidence_even_if_canon_is_released(self):
        coverage, registry, production = self.fixtures()
        coverage["sections"][0]["linked_requirement_ids"] = ["APGIC-BOOK-001"]
        registry["requirements"][1]["source_sections"] = [1, 2]
        for row in registry["requirements"]:
            row["status"] = "RELEASED"
        production = {"schema_version": "production-evidence-index-v1", "records": {"junk": {}}}
        result = inventory_tool.build_inventory(coverage, registry, production)
        self.assertEqual(result["totals"]["non_released_requirements"], 0)
        self.assertEqual(result["totals"]["owner_only_sections_needing_analysis"], 0)
        self.assertEqual(result["totals"]["traceability_mismatches"], 0)
        self.assertEqual(result["totals"]["production_evidence_records"], 1)
        manifest = ROOT / "config/release/r4-ci-gate-evidence.yaml"
        blockers = inventory_tool.strict_production_release_blockers(
            result, manifest, "a" * 40, production, registry,
        )
        self.assertTrue(any(reason.startswith("RELEASE_") for reason in blockers), blockers)
        self.assertTrue(any("SYNTHETIC_EVIDENCE_FORBIDDEN" in reason for reason in blockers), blockers)

    def test_refuses_missing_requirements_and_dependency_cycles(self):
        coverage, registry, production = self.fixtures()
        coverage["sections"][2]["linked_requirement_ids"] = ["APGIC-DOES-NOT-EXIST"]
        with self.assertRaises(inventory_tool.CanonInventoryError):
            inventory_tool.build_inventory(coverage, registry, production)
        coverage, registry, production = self.fixtures()
        registry["requirements"][1]["dependencies"] = ["APGIC-MISSING-001"]
        with self.assertRaisesRegex(inventory_tool.CanonInventoryError, "missing dependency"):
            inventory_tool.build_inventory(coverage, registry, production)
        coverage, registry, production = self.fixtures()
        registry["requirements"][1]["dependencies"] = ["APGIC-PAY-001"]
        with self.assertRaisesRegex(inventory_tool.CanonInventoryError, "cycle"):
            inventory_tool.build_inventory(coverage, registry, production)

    def test_full_frozen_v7_inventory_and_strict_complete_gate(self):
        coverage = json.loads((ROOT / "canon/requirements/coverage.json").read_text(encoding="utf-8"))
        registry = yaml.safe_load((ROOT / "canon/requirements/registry.yaml").read_text(encoding="utf-8"))
        production = yaml.safe_load((ROOT / "canon/evidence/production-evidence-index.yaml").read_text(encoding="utf-8"))
        result = inventory_tool.build_inventory(coverage, registry, production)
        self.assertEqual(result["totals"]["canon_sections"], len(coverage["sections"]))
        self.assertEqual(result["totals"]["requirements"], len(registry["requirements"]))
        self.assertEqual(
            sum(1 for section in result["section_index"] if section["tracking_class"] == "OWNER_ONLY_REQUIRES_ANALYSIS"),
            result["totals"]["owner_only_sections_needing_analysis"],
        )
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp)
            coverage_path, registry_path, production_path = (p / f"{k}.json" for k in ("coverage", "registry", "production"))
            output_path = p / "gap-inventory.json"
            coverage_path.write_text(json.dumps(coverage), encoding="utf-8")
            # YAML is also valid JSON; avoid recreating/rewriting normative source files.
            registry_path.write_text(json.dumps(registry), encoding="utf-8")
            production_path.write_text(json.dumps(production), encoding="utf-8")
            cmd = [
                sys.executable, str(ROOT / "tools/canon_execution_gap_inventory.py"),
                "--coverage", str(coverage_path), "--registry", str(registry_path),
                "--production-index", str(production_path),
                "--output", str(output_path),
            ]
            generated = subprocess.run(cmd, capture_output=True, text=True, check=False)
            self.assertEqual(generated.returncode, 0, generated.stderr)
            self.assertEqual(json.loads(output_path.read_text(encoding="utf-8")), result)
            forbidden_promotion = subprocess.run(cmd + ["--strict-complete"], capture_output=True, text=True, check=False)
            self.assertEqual(forbidden_promotion.returncode, 2)
            self.assertIn("BLOCKED", forbidden_promotion.stderr)


if __name__ == "__main__":
    unittest.main()
