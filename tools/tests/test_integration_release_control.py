import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("integration_release_control", ROOT / "tools/integration_release_control.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
PLAN = json.loads((ROOT / "config/release/integration-plan-v1.json").read_text())


def snapshot():
    return {"repository": PLAN["repository"], "candidate_ref": "feat/platform-readiness",
            "candidate_sha": "a" * 40, "baseline_sha": "b" * 40,
            "branches": {lane["branch"]: {"sha": (hex(i + 16)[2:] * 40)[:40],
                     "files_complete": True, "files": [], "included_in_candidate": True}
                     for i, lane in enumerate(PLAN["lanes"])}}


def evidence():
    return {"candidate_sha": "a" * 40, "launch_ready": True, "blockers": [],
            "promotion_authorized": False,
            "evidence_class": "CANDIDATE_ASSESSMENT_NOT_RELEASE_EVIDENCE",
            "evidence": {"production_index_records": 9, "r4_production_manifest_supplied": True,
                         "production_config_preflight_supplied": True,
                         "critical_journey_manifest_supplied": True}}


class IntegrationControlTests(unittest.TestCase):
    def test_baseline_requires_real_launch_assessment(self):
        result = module.assessment(PLAN, snapshot())
        self.assertEqual(result["decision"], "BLOCKED")
        self.assertIn("CANON_LAUNCH_ASSESSMENT_MISSING", {b["code"] for b in result["launch_blockers"]})

    def test_collision_postgres_serial_version_even_when_unique_file_names(self):
        state = snapshot()
        state["branches"]["feat/specialist-marketplace"]["files"] = ["backend/migrations/000032_specialist.sql"]
        state["branches"]["feat/consultation-runtime"]["files"] = ["backend/migrations/000032_consultation.sql"]
        result = module.assessment(PLAN, state, evidence())
        collisions = [b for b in result["launch_blockers"] if b["code"] == "POSTGRES_MIGRATION_VERSION_COLLISION"]
        self.assertEqual(len(collisions), 1)
        self.assertEqual(collisions[0]["owner"], "postgres-schema-owner")
        self.assertEqual(len(collisions[0]["evidence"]), 2)
        self.assertEqual(result["decision"], "BLOCKED")

    def test_shared_api_and_generated_types_detected(self):
        state = snapshot()
        paths = ["contracts/openapi/apgic-v1.yaml", "packages/contracts/src/generated/apgic-v1.ts"]
        for name in ("feat/booking-complete", "feat/specialist-marketplace"):
            state["branches"][name]["files"] = paths
        result = module.assessment(PLAN, state, evidence())
        self.assertTrue({"OPENAPI_CONTRACT_COLLISION", "GENERATED_CLIENT_COLLISION"} <=
                        {b["code"] for b in result["launch_blockers"]})

    def test_post_launch_does_not_expand_launch_cut(self):
        state = snapshot()
        state["branches"]["feat/academy-edtech"]["included_in_candidate"] = False
        state["branches"]["feat/groups-live-events"]["included_in_candidate"] = False
        result = module.assessment(PLAN, state, evidence())
        self.assertEqual(result["decision"], "READY_FOR_SEPARATE_PROMOTION_REVIEW")
        self.assertEqual(result["promotion_authorized"], False)
        self.assertGreaterEqual(len(result["post_launch_blockers"]), 2)

    def test_unmerged_launch_branch_blocks_release(self):
        state = snapshot()
        state["branches"]["feat/booking-complete"]["included_in_candidate"] = False
        result = module.assessment(PLAN, state, evidence())
        self.assertEqual(result["decision"], "BLOCKED")
        self.assertTrue(any(b["owner"] == "booking-complete" for b in result["launch_blockers"]))

    def test_unseen_files_never_treated_as_conflict_free(self):
        state = snapshot()
        state["branches"]["feat/booking-complete"]["files_complete"] = False
        result = module.assessment(PLAN, state, evidence())
        self.assertIn("BRANCH_CHANGESET_INCOMPLETE", {b["code"] for b in result["launch_blockers"]})

    def test_wrong_gate_candidate_sha_blocks(self):
        proof = evidence()
        proof["candidate_sha"] = "c" * 40
        result = module.assessment(PLAN, snapshot(), proof)
        self.assertIn("CANON_LAUNCH_ASSESSMENT_SHA_MISMATCH", {b["code"] for b in result["launch_blockers"]})

    def test_never_downgrade_strict_gate_blockers(self):
        proof = evidence()
        proof["blockers"] = ["R4_EXTERNAL_PRODUCTION_EVIDENCE_MANIFEST_MISSING"]
        proof["launch_ready"] = False
        result = module.assessment(PLAN, snapshot(), proof)
        self.assertEqual(result["decision"], "BLOCKED")
        self.assertIn("CANON_R4_EXTERNAL_PRODUCTION_EVIDENCE_MANIFEST_MISSING", {b["code"] for b in result["launch_blockers"]})

    def test_live_snapshot_checks_head_ancestry_and_changed_paths(self):
        import unittest.mock as mock
        state = snapshot()
        names = [lane["branch"] for lane in PLAN["lanes"]]
        # No network: simulate exact GitHub REST payloads and verify we check
        # ancestry rather than treating a green branch CI as integrated.
        def fake_github(repo, endpoint, token):
            if endpoint.startswith("branches/"):
                branch = endpoint.split("branches/", 1)[1].replace("%2F", "/")
                if branch == "main":
                    return {"commit": {"sha": "b" * 40}}
                if branch == "feat/platform-readiness":
                    return {"commit": {"sha": "a" * 40}}
                return {"commit": {"sha": state["branches"][branch]["sha"]}}
            if endpoint.startswith("compare/"):
                ref = endpoint.split("compare/", 1)[1].split("?", 1)[0]
                left, right = ref.split("...", 1)
                if left == "b" * 40:
                    return {"files": [{"filename": "backend/migrations/000032_branch.sql"}], "ahead_by": 1}
                return {"status": "behind", "behind_by": 1}
            raise AssertionError(endpoint)
        with mock.patch.object(module, "github_json", side_effect=fake_github):
            live = module.live_snapshot(PLAN, "feat/platform-readiness", "test-token")
        self.assertEqual(live["candidate_sha"], "a" * 40)
        self.assertEqual(set(live["branches"]), set(names))
        self.assertFalse(live["branches"]["feat/booking-complete"]["included_in_candidate"])
        self.assertEqual(len(live["branches"]["feat/booking-complete"]["files"]), 1)

    def test_report_contains_named_owners_and_pinned_code_evidence(self):
        state = snapshot()
        state["branches"]["feat/specialist-marketplace"]["files"] = ["backend/migrations/000032_a.sql"]
        state["branches"]["feat/consultation-runtime"]["files"] = ["backend/migrations/000032_b.sql"]
        report = module.assessment(PLAN, state)
        text = module.markdown(report)
        self.assertIn("postgres-schema-owner", text)
        self.assertIn("https://github.com/", text)
        self.assertNotIn("**READY_FOR_SEPARATE_PROMOTION_REVIEW**", text)


if __name__ == "__main__":
    unittest.main()
