"""Isolated local Git proof of branch inventory semantics (no network)."""
from __future__ import annotations

import importlib.util
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

MODULE = Path(__file__).resolve().parents[1] / "branch_consolidation.py"
SPEC = importlib.util.spec_from_file_location("branch_consolidation", MODULE)
assert SPEC and SPEC.loader
tool = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tool)


class BranchInventoryTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.old = Path.cwd()
        os.chdir(self.root)
        self.addCleanup(os.chdir, self.old)
        self.cmd("git", "init", "-q", "-b", "main")
        self.cmd("git", "config", "user.email", "ci@example.invalid")
        self.cmd("git", "config", "user.name", "CI")
        self.write("backend/migrations/000001_base.sql", "select 1;\n")
        self.commit("base")
        self.cmd("git", "remote", "add", "origin", ".")
        self.cmd("git", "update-ref", "refs/remotes/origin/main", self.rev())
        self.cmd("git", "branch", "old", self.rev())
        self.cmd("git", "branch", "feature", self.rev())
        self.cmd("git", "checkout", "-q", "feature")
        self.write("backend/migrations/000001_conflict.sql", "select 2;\n")
        self.write("backend/internal/new_feature.go", "package internal\n")
        self.commit("feature")
        self.cmd("git", "update-ref", "refs/remotes/origin/feature", self.rev())
        self.cmd("git", "checkout", "-q", "main")
        self.cmd("git", "update-ref", "refs/remotes/origin/old", self.rev())

    def cmd(self, *args: str) -> str:
        return subprocess.check_output(args, text=True).strip()

    def write(self, path: str, content: str) -> None:
        output = self.root / path
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(content, encoding="utf-8")

    def commit(self, msg: str) -> None:
        self.cmd("git", "add", ".")
        self.cmd("git", "commit", "-qm", msg)

    def rev(self) -> str:
        return self.cmd("git", "rev-parse", "HEAD")

    def test_classifies_ancestor_and_divergent_without_claiming_feature_absence(self) -> None:
        report = tool.inventory()
        self.assertEqual((report["total_non_main"], report["ancestor_branches"], report["divergent_branches"]), (2, 1, 1))
        rows = {x["branch"]: x for x in report["branches"]}
        self.assertEqual(rows["origin/old"]["classification"], "ANCESTOR")
        feature = rows["origin/feature"]
        self.assertEqual(feature["classification"], "DIVERGENT_REQUIRES_SEMANTIC_REVIEW")
        self.assertIn("backend/internal/new_feature.go", feature["introduced_paths_absent_from_main"])
        self.assertEqual(feature["migration_number_conflicts"][0]["version"], "000001")
        self.assertEqual(feature["migration_number_conflicts"][0]["main_path"], "backend/migrations/000001_base.sql")

    def test_rejects_bad_remote_and_reports_baseline_collisions(self) -> None:
        with self.assertRaises(ValueError):
            tool.inventory(remote="origin;rm -rf")
        self.write("backend/migrations/000001_duplicate.sql", "select 3;\n")
        self.commit("duplicate")
        self.cmd("git", "update-ref", "refs/remotes/origin/main", self.rev())
        report = tool.inventory()
        self.assertEqual(report["baseline_migration_number_collisions"][0]["version"], "000001")
        self.assertEqual(len(report["baseline_migration_number_collisions"][0]["paths"]), 2)
        self.assertEqual(len(report["branches"]), 2)


if __name__ == "__main__":
    unittest.main()
