"""Local bare-origin proof of guarded branch cleanup; never contacts GitHub."""
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from tools import branch_prune_ancestry as sut


class BranchPruneAncestryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.dir = Path(self.temp.name)
        self.bare = self.dir / "origin.git"
        self.work = self.dir / "work"
        subprocess.run(["git", "init", "--bare", "-q", str(self.bare)], check=True)
        subprocess.run(["git", "init", "-q", "-b", "main", str(self.work)], check=True)
        self.oldcwd = Path.cwd()
        os.chdir(self.work)
        self.addCleanup(os.chdir, self.oldcwd)
        self.cmd("git", "config", "user.name", "test")
        self.cmd("git", "config", "user.email", "test@example.invalid")
        self.cmd("git", "remote", "add", "origin", str(self.bare))
        (self.work / "file.txt").write_text("base\n")
        self.cmd("git", "add", ".")
        self.cmd("git", "commit", "-qm", "base")
        self.base = self.cmd("git", "rev-parse", "HEAD")
        self.cmd("git", "push", "-q", "origin", "main")
        self.cmd("git", "branch", "integrated")
        self.cmd("git", "push", "-q", "origin", "integrated")
        self.cmd("git", "checkout", "-q", "-b", "divergent")
        (self.work / "other.txt").write_text("different\n")
        self.cmd("git", "add", ".")
        self.cmd("git", "commit", "-qm", "diverged")
        self.divergent = self.cmd("git", "rev-parse", "HEAD")
        self.cmd("git", "push", "-q", "origin", "divergent")
        self.report = {"baseline_sha": self.base, "branches": [
            {"branch":"origin/integrated", "sha":self.base, "ahead":0, "classification":"ANCESTOR"},
            {"branch":"origin/divergent", "sha":self.divergent, "ahead":1, "classification":"DIVERGENT_REQUIRES_SEMANTIC_REVIEW"}]}

    def cmd(self, *command):
        return subprocess.check_output(command, text=True).strip()

    def test_dry_run_and_authenticated_local_bare_delete_only_ancestor(self):
        dry = sut.prune(self.report, "origin", False, None)
        self.assertEqual(dry["eligible"], 1)
        self.assertEqual(dry["deleted"], [])
        self.assertEqual(sut.remote_sha("origin", "integrated"), self.base)
        with self.assertRaisesRegex(ValueError, "--confirm-main"):
            sut.prune(self.report, "origin", True, "0"*40)
        applied = sut.prune(self.report, "origin", True, self.base)
        self.assertEqual(applied["deleted"], ["integrated"])
        self.assertIsNone(sut.remote_sha("origin", "integrated"))
        self.assertEqual(sut.remote_sha("origin", "divergent"), self.divergent)
        self.assertEqual(sut.remote_sha("origin", "main"), self.base)

    def test_changed_branch_or_main_cannot_be_deleted(self):
        (self.work / "file.txt").write_text("main updated\n")
        self.cmd("git", "add", ".")
        self.cmd("git", "commit", "-qm", "new")
        self.cmd("git", "push", "-q", "origin", "HEAD:integrated")
        dry = sut.prune(self.report, "origin", False, None)
        self.assertEqual(dry["eligible"], 0)
        self.assertEqual(sut.remote_sha("origin", "integrated"), self.cmd("git", "rev-parse", "HEAD"))
        self.cmd("git", "push", "-q", "origin", "HEAD:main")
        with self.assertRaisesRegex(ValueError, "remote main changed"):
            sut.prune(self.report, "origin", True, self.base)

    def test_rejects_main_and_wrong_remote(self):
        bad = dict(self.report, branches=[{"branch":"origin/main","sha":self.base,
                     "ahead":0,"classification":"ANCESTOR"}])
        with self.assertRaisesRegex(ValueError, "protected branch"):
            sut.prune(bad, "origin", False, None)
        with self.assertRaises(subprocess.CalledProcessError):
            sut.prune(self.report, "backup", False, None)


if __name__ == "__main__":
    unittest.main()
