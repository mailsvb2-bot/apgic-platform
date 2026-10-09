from __future__ import annotations

import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

import yaml


SCRIPT = Path(__file__).resolve().parents[1] / "production_preflight_evidence.py"
SPEC = importlib.util.spec_from_file_location("production_preflight_evidence", SCRIPT)
assert SPEC and SPEC.loader
module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(module)


class ProductionPreflightEvidenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        self.root = Path(self.tempdir.name)
        self.original_root = module.ROOT
        module.ROOT = self.root
        self.addCleanup(setattr, module, "ROOT", self.original_root)

    def write_yaml(self, rel: str, data: dict) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(yaml.safe_dump(data, sort_keys=True), encoding="utf-8")

    def write_bytes(self, rel: str, data: bytes) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def seed_canonical_binding(self) -> None:
        canon_bytes = b"canonical-v7-final-test"
        canon_sha = hashlib.sha256(canon_bytes).hexdigest()
        meta = {
            "registry_version": "1.0",
            "canon_execution_baseline": "v7-final-qa-strict-non-custodial",
            "canonical_docx_sha256": canon_sha,
        }
        self.write_yaml("canon/requirements/registry.yaml", {"meta": meta, "requirements": []})
        coverage = self.root / "canon/requirements/coverage.json"
        coverage.parent.mkdir(parents=True, exist_ok=True)
        coverage.write_text(json.dumps({"meta": meta, "sections": []}), encoding="utf-8")
        self.write_yaml("canon/requirements/approved-rfcs.yaml", {"rfcs": []})
        self.write_yaml("canon/baseline/APGIC_Requirement_Registry_v7_FINAL.yaml", {"meta": meta})
        baseline_coverage = self.root / "canon/baseline/APGIC_Canon_Coverage_v7_FINAL.json"
        baseline_coverage.parent.mkdir(parents=True, exist_ok=True)
        baseline_coverage.write_text(json.dumps({"meta": meta}), encoding="utf-8")
        self.write_bytes(
            "canon/APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx",
            canon_bytes,
        )
        self.write_yaml("canon/contracts/provider-terminology.yaml", {"version": "v1"})
        self.write_bytes("contracts/openapi/apgic-v1.yaml", b"openapi: 3.1.0\n")
        self.write_bytes("contracts/jsonschema/test.schema.json", b'{"type":"object"}\n')
        self.write_yaml("contracts/testing/state-machine.yaml", {"version": "v1"})

    def production_config(self) -> dict:
        config = {
            "environment": "PRODUCTION",
            "production_approved": True,
        }
        for field in module.PROFILE_POLICY_FIELDS["R0"]:
            rel = f"config/policies/{field}.yaml"
            self.write_yaml(rel, {"version": field + "-v1"})
            config[field] = rel
        return config

    def test_build_evidence_binds_exact_candidate_and_policy_hashes(self) -> None:
        self.seed_canonical_binding()
        self.write_yaml("config/launch.production.yaml", self.production_config())

        evidence = module.build_evidence(
            "a" * 40,
            "R0",
            "config/launch.production.yaml",
        )

        self.assertEqual(evidence["schema_version"], "production-preflight-evidence-v1")
        self.assertEqual(evidence["candidate_sha"], "a" * 40)
        self.assertEqual(evidence["release_profile"], "R0")
        self.assertTrue(evidence["production_promotion_eligible"])
        self.assertFalse(evidence["production_release"])
        self.assertEqual(
            set(evidence["policy_artifacts"]),
            set(module.PROFILE_POLICY_FIELDS["R0"]),
        )
        for row in evidence["policy_artifacts"].values():
            self.assertEqual(len(row["sha256"]), 64)
        binding = evidence["canonical_binding"]
        self.assertEqual(binding["registry_version"], "1.0")
        self.assertEqual(binding["canon_execution_baseline"], "v7-final-qa-strict-non-custodial")
        self.assertEqual(len(binding["requirement_registry_revision"]), 64)
        self.assertEqual(len(binding["contract_schema_registry_revision"]), 64)
        self.assertIn("canon/requirements/registry.yaml", binding["requirement_artifacts"])
        self.assertIn("contracts/openapi/apgic-v1.yaml", binding["contract_schema_artifacts"])

    def test_ci_only_or_unapproved_config_cannot_create_production_evidence(self) -> None:
        self.seed_canonical_binding()
        config = self.production_config()
        config["environment"] = "CI"
        config["production_approved"] = False
        self.write_yaml("config/launch.ci.yaml", config)

        with self.assertRaises(SystemExit):
            module.build_evidence("b" * 40, "R0", "config/launch.ci.yaml")

    def test_candidate_sha_must_be_exact_lowercase_git_sha(self) -> None:
        self.seed_canonical_binding()
        self.write_yaml("config/launch.production.yaml", self.production_config())

        for candidate in ("abc", "A" * 40, "g" * 40):
            with self.subTest(candidate=candidate), self.assertRaises(SystemExit):
                module.build_evidence(candidate, "R0", "config/launch.production.yaml")



    def test_canonical_docx_mismatch_fails_closed(self) -> None:
        self.seed_canonical_binding()
        self.write_bytes(
            "canon/APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx",
            b"tampered",
        )
        self.write_yaml("config/launch.production.yaml", self.production_config())

        with self.assertRaises(SystemExit):
            module.build_evidence("c" * 40, "R0", "config/launch.production.yaml")

    def test_contract_revision_changes_when_contract_bytes_change(self) -> None:
        self.seed_canonical_binding()
        first = module.build_canonical_binding()["contract_schema_registry_revision"]
        self.write_bytes("contracts/jsonschema/test.schema.json", b'{"type":"string"}\n')
        second = module.build_canonical_binding()["contract_schema_registry_revision"]
        self.assertNotEqual(first, second)


if __name__ == "__main__":
    unittest.main()
