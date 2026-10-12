"""True isolated 3-way Git merge, migration ledger and non-mutation regression tests."""
import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("integration_candidate", ROOT / "tools/integration_candidate.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args],
                                   text=True, stderr=subprocess.STDOUT).strip()

class AssemblyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / "repo"
        self.repo.mkdir()
        git(self.repo, "init", "-b", "main")
        git(self.repo, "config", "user.name", "Fixture Owner")
        git(self.repo, "config", "user.email", "test@example.invalid")
        (self.repo / "backend/migrations").mkdir(parents=True)
        (self.repo / "backend/migrations/000001_base.sql").write_text("select 1;\n")
        (self.repo / "shared.txt").write_text("initial\n")
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-m", "base")
        self.base = git(self.repo, "rev-parse", "HEAD")
        self.plan = {
            "baseline": "main", "control_branch": "feat/platform-readiness",
            "lanes": [
                {"order":0,"branch":"feat/platform-readiness","owner":"readiness","profile":"R0","launch_dependency":True,"dependencies":[]},
                {"order":1,"branch":"feat/marketplace","owner":"marketplace","profile":"R1","launch_dependency":True,"dependencies":[]},
                {"order":2,"branch":"feat/booking","owner":"booking","profile":"R2","launch_dependency":True,"dependencies":["feat/marketplace"]},
                {"order":3,"branch":"feat/academy","owner":"academy","profile":"R8","launch_dependency":False,"dependencies":["feat/booking"]}]}
        for name, file, content in [
            ("feat/platform-readiness","control.txt","control\n"),
            ("feat/marketplace","backend/migrations/000002_marketplace.sql","select 2;\n"),
            ("feat/booking","booking.txt","booking\n"),
            ("feat/academy","academy.txt","academy\n")]:
            git(self.repo, "checkout", "-b", name, "main")
            path = self.repo / file
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
            git(self.repo, "add", ".")
            git(self.repo, "commit", "-m", name)
        git(self.repo,"checkout","main")
        for lane in self.plan["lanes"]:
            name = lane["branch"]
            git(self.repo,"update-ref","refs/remotes/origin/" + name,git(self.repo,"rev-parse",name))
        git(self.repo,"update-ref","refs/remotes/origin/main",self.base)

    def update(self, branch, path, content):
        git(self.repo,"checkout",branch)
        dest = self.repo / path
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(content)
        git(self.repo,"add",".")
        git(self.repo,"commit","-m","feature overlap")
        git(self.repo,"update-ref","refs/remotes/origin/" + branch,git(self.repo,"rev-parse","HEAD"))
        git(self.repo,"checkout","main")

    def test_genuine_merges_preserve_all_source_heads(self):
        before = git(self.repo, "rev-parse", "feat/platform-readiness")
        result = module.assemble(self.plan, self.repo)
        self.assertTrue(result["composition_complete"])
        self.assertEqual([s["status"] for s in result["stages"]],["MERGED_FOR_TESTING"]*3)
        self.assertEqual(result["production_release"],False)
        self.assertEqual(before,git(self.repo,"rev-parse","feat/platform-readiness"))
        self.assertEqual(len(result["pairwise_probes"]),3)

    def test_actual_git_conflict_and_dependent_stage_held(self):
        self.update("feat/marketplace","shared.txt","marketplace\n")
        self.update("feat/booking","shared.txt","booking\n")
        report = module.assemble(self.plan, self.repo, scope="all")
        self.assertEqual(report["stages"][2]["status"],"MERGE_CONFLICT")
        self.assertIn("shared.txt", report["stages"][2]["blockers"][0]["files"])
        self.assertEqual(report["stages"][3]["status"],"NOT_EXECUTED")
        self.assertEqual(report["pairwise_probes"][2]["status"],"MERGES_WITH_CONTROL_ALONE")

    def test_real_database_migration_number_collision(self):
        self.update("feat/booking","backend/migrations/000002_booking.sql","select 3;\n")
        result = module.assemble(self.plan,self.repo,scope="all")
        self.assertEqual(result["stages"][2]["status"],"POSTGRES_MIGRATION_COLLISION")
        self.assertEqual(result["stages"][2]["blockers"][0]["version"],"000002")
        self.assertEqual(result["stages"][3]["status"],"NOT_EXECUTED")

    def test_release_cut_excludes_future_lanes(self):
        result = module.assemble(self.plan,self.repo)
        self.assertEqual(len(result["stages"]),3)
        self.assertEqual(result["pairwise_probes"][-1]["branch"],"feat/academy")

    def test_retained_worktree_is_a_build_tree_not_remote_branch(self):
        dest = Path(self.temp.name) / "composed"
        result = module.assemble(self.plan,self.repo,retain=dest)
        self.assertTrue((dest / "booking.txt").exists())
        self.assertFalse(result["persisted_remote_branch"])
        git(self.repo,"worktree","remove","--force",str(dest))

    def test_missing_ref_blocks_without_guessing(self):
        git(self.repo,"update-ref","-d","refs/remotes/origin/feat/booking")
        with self.assertRaisesRegex(ValueError,"missing ref|Unpinned"):
            module.assemble(self.plan,self.repo)

if __name__ == "__main__":
    unittest.main()
