import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]


class CanonLaunchFreezeE2ETest(unittest.TestCase):
    def test_unapproved_launch_requirement_is_rejected_by_real_canon_lint(self):
        with tempfile.TemporaryDirectory(prefix="apgic-canon-freeze-") as tmp:
            sandbox = pathlib.Path(tmp) / "repo"
            shutil.copytree(
                ROOT,
                sandbox,
                ignore=shutil.ignore_patterns(
                    ".git",
                    "node_modules",
                    ".next",
                    "evidence",
                    "__pycache__",
                    "*.pyc",
                ),
            )

            registry_path = sandbox / "canon/requirements/registry.yaml"
            registry = yaml.safe_load(registry_path.read_text(encoding="utf-8"))
            requirements = registry.get("requirements") or []
            template = dict(requirements[0])
            template["requirement_id"] = "APGIC-TESTFREEZE-999"
            template["title"] = "Unauthorized launch scope expansion probe"
            template["statement"] = "Synthetic requirement that must never enter Launch Cut without governed Canon change."
            template["owner_domain"] = "GOVERNANCE"
            template["source_sections"] = [999]
            template["release_profile"] = "R0"
            template["status"] = "PROPOSED"
            template["dependencies"] = []
            template["implementation_refs"] = []
            template["contract_refs"] = []
            template["test_refs"] = []
            template["evidence_refs"] = []
            template["acceptance"] = {
                "given": "an unapproved feature idea",
                "when": "it is inserted into the R0 Requirement Registry",
                "then": "the executable Canon rejects the scope expansion",
                "required_evidence": ["E2E_OR_STAGING_PROOF"],
            }
            requirements.append(template)
            registry_path.write_text(
                yaml.safe_dump(registry, allow_unicode=True, sort_keys=False),
                encoding="utf-8",
            )

            result = subprocess.run(
                [sys.executable, "tools/canon_lint.py"],
                cwd=sandbox,
                text=True,
                capture_output=True,
                timeout=60,
                check=False,
            )

            output = result.stdout + result.stderr
            self.assertNotEqual(result.returncode, 0, output)
            self.assertIn(
                "working registry requirement set differs from immutable baseline",
                output,
            )


if __name__ == "__main__":
    unittest.main()
