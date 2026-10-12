"""Fail-closed production-launch assessment tests, including real frozen inputs."""
from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import yaml

SCRIPT = Path(__file__).resolve().parents[1] / "launch_readiness.py"
SPEC = importlib.util.spec_from_file_location("launch_readiness", SCRIPT)
assert SPEC and SPEC.loader
import sys
sys.path.insert(0, str(SCRIPT.parent))
launch = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(launch)


class LaunchReadinessTests(unittest.TestCase):
    def setUp(self) -> None:
        self.sha = "a" * 40
        self.meta = {"version": "v7_FINAL", "canonical_docx_sha256": "d" * 64}
        self.row = {
            "requirement_id": "APGIC-EXEC-001", "title": "Traceability",
            "owner_domain": "EXEC", "release_profile": "R0", "priority": "P0",
            "status": "RELEASED", "source_sections": [1], "dependencies": [],
            "implementation_refs": ["backend/internal/example.go"],
            "contract_refs": ["contracts/example.yaml"],
            "test_refs": ["backend/internal/example_test.go"],
            "evidence_refs": ["evidence://proof"],
            "acceptance": {"required_evidence": ["DOMAIN_OR_CONTRACT_TEST"]},
        }
        self.coverage = {"meta": dict(self.meta), "sections": [
            {"section": 1, "title": "Test", "owner_domain": "EXEC",
             "coverage_status": "EXPLICIT_REQUIREMENT_LINK",
             "linked_requirement_ids": ["APGIC-EXEC-001"]},
        ]}
        self.registry = {"meta": dict(self.meta), "requirements": [dict(self.row)]}
        self.index = {"schema_version": "production-evidence-index-v1", "records": {
            "proof": {"evidence_type": "PROOF", "status": "PASS"}
        }}
        self.preflight = {"candidate_sha": self.sha, "release_profile": "R4",
                          "production_release": False, "evidence_class": "PRODUCTION_PREFLIGHT"}
        self.manifest = {"candidate_sha": self.sha, "synthetic": False,
                         "production_candidate": True, "schema_version": "r4-release-gate-v1",
                         "evidence": {}}

    def evaluate(self, **changes):
        params = {"candidate_sha": self.sha, "coverage": self.coverage,
                  "registry": self.registry, "production_index": self.index,
                  "manifest": self.manifest, "preflight_evidence": self.preflight}
        params.update(changes)
        # The existing R4 gate remains authoritative; here only substitute its
        # outcome to isolate the launch-cut policy under test.
        with patch.object(launch.r4, "evaluate", return_value=(True, [])), \
             patch.object(launch.r4, "canon_dependency_blockers", return_value=[]):
            return launch.assess(**params)

    def test_only_complete_launch_proof_passes_prelaunch_decision(self):
        report = self.evaluate()
        self.assertTrue(report["launch_ready"])
        self.assertFalse(report["promotion_authorized"])
        self.assertEqual(report["requirements"]["launch_total"], 1)
        self.assertEqual(report["blockers"], [])

    def test_missing_production_manifest_is_not_ci_success(self):
        report = self.evaluate(manifest=None)
        self.assertFalse(report["launch_ready"])
        self.assertIn("R4_EXTERNAL_PRODUCTION_EVIDENCE_MANIFEST_MISSING", report["blockers"])

    def test_empty_production_index_fails_closed(self):
        report = self.evaluate(production_index={"schema_version": "production-evidence-index-v1", "records": {}})
        self.assertIn("PRODUCTION_EVIDENCE_INDEX_EMPTY", report["blockers"])

    def test_unreleased_requirement_is_blocker_despite_full_refs(self):
        self.registry["requirements"][0]["status"] = "VERIFIED"
        report = self.evaluate()
        self.assertIn("LAUNCH_REQUIREMENTS_UNRELEASED_OR_UNPROVEN", report["blockers"])
        self.assertIn("REQUIREMENT_NOT_RELEASED", report["requirements"]["blocked"][0]["reasons"])

    def test_untraced_release_status_is_blocker(self):
        self.registry["requirements"][0]["test_refs"] = []
        report = self.evaluate()
        self.assertIn("TEST_REFS_MISSING", report["requirements"]["blocked"][0]["reasons"])

    def test_post_launch_requirement_not_falsely_launch_critical(self):
        self.registry["requirements"].append({**self.row, "requirement_id": "APGIC-R5-001",
                                              "release_profile": "R5", "source_sections": [],
                                              "status": "IN_PROGRESS"})
        report = self.evaluate()
        self.assertTrue(report["launch_ready"])
        self.assertEqual(report["governance"]["post_launch_requirements_excluded_from_launch_cut"], 1)

    def test_owner_only_sections_are_governance_review_not_fake_missing_feature(self):
        self.coverage["sections"].append({
            "section": 2, "title": "Postlaunch description", "owner_domain": "GROWTH",
            "coverage_status": "SECTION_OWNER_BASELINE", "linked_requirement_ids": [],
        })
        report = self.evaluate()
        self.assertTrue(report["launch_ready"])
        self.assertEqual(len(report["warnings"]), 1)

    def test_mismatch_between_frozen_coverage_and_registry_blocks(self):
        self.coverage["sections"][0]["linked_requirement_ids"] = []
        report = self.evaluate()
        self.assertIn("FROZEN_CANON_LINKAGE_MISMATCH", report["blockers"])

    def test_synthetic_or_other_candidate_rejected(self):
        self.manifest["synthetic"] = True
        self.manifest["candidate_sha"] = "b" * 40
        report = self.evaluate()
        self.assertIn("R4_MANIFEST_NOT_REAL_PRODUCTION", report["blockers"])
        self.assertIn("R4_MANIFEST_CANDIDATE_MISMATCH", report["blockers"])

    def test_bad_sha_and_preflight_binding_rejected(self):
        self.preflight["candidate_sha"] = "c" * 40
        report = self.evaluate(candidate_sha="short")
        self.assertIn("CANDIDATE_SHA_NOT_EXACT", report["blockers"])
        self.assertIn("PRODUCTION_CONFIG_CANDIDATE_MISMATCH", report["blockers"])

    def test_r4_validator_blockers_survive_even_with_released_requirements(self):
        with patch.object(launch.r4, "evaluate", return_value=(False, ["REAL_PROVIDER_PROOF_MISSING"])), \
             patch.object(launch.r4, "canon_dependency_blockers", return_value=[]):
            report = launch.assess(candidate_sha=self.sha, coverage=self.coverage,
                 registry=self.registry, production_index=self.index, manifest=self.manifest,
                 preflight_evidence=self.preflight)
        self.assertIn("R4_REAL_PROVIDER_PROOF_MISSING", report["blockers"])

    def test_real_frozen_baseline_must_not_claim_ready(self):
        root = SCRIPT.parents[2]
        registry = yaml.safe_load((root / "canon/requirements/registry.yaml").read_text(encoding="utf-8"))
        coverage = json.loads((root / "canon/requirements/coverage.json").read_text(encoding="utf-8"))
        idx = yaml.safe_load((root / "canon/evidence/production-evidence-index.yaml").read_text(encoding="utf-8"))
        report = launch.assess(candidate_sha=self.sha, coverage=coverage,
              registry=registry, production_index=idx)
        self.assertFalse(report["launch_ready"])
        self.assertGreater(len(report["requirements"]["blocked"]), 0)
        self.assertIn("PRODUCTION_EVIDENCE_INDEX_EMPTY", report["blockers"])
        self.assertIn("R4_EXTERNAL_PRODUCTION_EVIDENCE_MANIFEST_MISSING", report["blockers"])


if __name__ == "__main__":
    unittest.main()
