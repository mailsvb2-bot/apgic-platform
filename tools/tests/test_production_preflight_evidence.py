from __future__ import annotations

import importlib.util
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

    def test_ci_only_or_unapproved_config_cannot_create_production_evidence(self) -> None:
        config = self.production_config()
        config["environment"] = "CI"
        config["production_approved"] = False
        self.write_yaml("config/launch.ci.yaml", config)

        with self.assertRaises(SystemExit):
            module.build_evidence("b" * 40, "R0", "config/launch.ci.yaml")

    def test_candidate_sha_must_be_exact_lowercase_git_sha(self) -> None:
        self.write_yaml("config/launch.production.yaml", self.production_config())

        for candidate in ("abc", "A" * 40, "g" * 40):
            with self.subTest(candidate=candidate), self.assertRaises(SystemExit):
                module.build_evidence(candidate, "R0", "config/launch.production.yaml")


if __name__ == "__main__":
    unittest.main()
