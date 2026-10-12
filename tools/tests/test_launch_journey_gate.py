from __future__ import annotations

import hashlib
import json
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from tools import launch_journey_gate as gate

CANDIDATE = "a" * 40
OBSERVED = "2026-10-10T12:00:00Z"
NOW = datetime(2026, 10, 12, tzinfo=timezone.utc)


class LaunchJourneyGateTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)

    def complete(self):
        manifest = {"schema_version": "launch-critical-journeys-v1", "candidate_sha": CANDIDATE,
                    "synthetic": False, "environment": "PRODUCTION", "journeys": {}}
        index = {"schema_version": "production-evidence-index-v1", "records": {}}
        for name, (_, phases) in gate.JOURNEYS.items():
            evid = "journeys/" + name
            path = f"canon/evidence/production/journeys/{name}.json"
            source = "https://proofs.apgic.ru/releases/" + name
            correlation = name + "-correlation"
            observation = {
                "schema_version": "launch-journey-observation-v1",
                "journey_id": name, "candidate_sha": CANDIDATE,
                "environment": "PRODUCTION", "status": "PASS", "synthetic": False,
                "source_ref": source, "verified_at": OBSERVED,
                "verified_by": "independent-verifier", "correlation_id": correlation,
                "observed_steps": [
                    {"phase": phase, "event_id": name + str(i), "canonical_owner": "APGIC",
                     "correlation_id": correlation,
                     "observed_at": OBSERVED} for i, phase in enumerate(phases)
                ],
                "business_outcome": {"status": "CONFIRMED", "canonical_result_id": name + "-outcome",
                                     "correlation_id": correlation},
                "failure_or_denial_probe": {"status": "PASS",
                    "correlation_id": correlation,
                    "observed_result": "recovered or rejected safely",
                    "source_ref": source + "/denial"},
            }
            target = self.root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(json.dumps(observation), encoding="utf-8")
            index["records"][evid] = {
                "evidence_type": "LAUNCH_JOURNEY_E2E", "journey_id": name,
                "candidate_sha": CANDIDATE, "status": "PASS", "synthetic": False,
                "source_ref": source, "verified_at": OBSERVED,
                "correlation_id": correlation, "artifact_path": path,
                "artifact_sha256": hashlib.sha256(target.read_bytes()).hexdigest(),
            }
            manifest["journeys"][name] = {"evidence_ref": "evidence://" + evid}
        return manifest, index

    def blocked(self, manifest, index, reason):
        ok, failures = gate.evaluate(manifest, index, CANDIDATE, root=self.root, now=NOW)
        self.assertFalse(ok)
        self.assertTrue(any(reason in f for f in failures), failures)

    def test_complete_observations_are_admissible_not_launch_authorization(self):
        manifest, idx = self.complete()
        self.assertEqual(gate.evaluate(manifest, idx, CANDIDATE, root=self.root, now=NOW), (True, []))

    def test_missing_manifest_fails_closed(self):
        self.blocked(None, {}, "MANIFEST_MISSING")

    def test_missing_journey_and_synthetic_blocked(self):
        manifest, idx = self.complete()
        manifest["journeys"].pop("r0-isolated-restore")
        manifest["synthetic"] = True
        self.blocked(manifest, idx, "JOURNEY_SET_INCOMPLETE")
        self.blocked(manifest, idx, "PRODUCTION_PROOF_REQUIRED")

    def test_reused_evidence_not_counted_twice(self):
        manifest, idx = self.complete()
        manifest["journeys"]["r4-native-store"] = manifest["journeys"]["r4-provider-settlement"]
        self.blocked(manifest, idx, "PROOF_REUSED")

    def test_changed_candidate_rejected(self):
        manifest, idx = self.complete()
        manifest["candidate_sha"] = "b" * 40
        self.blocked(manifest, idx, "CANDIDATE_SHA_MISMATCH")

    def test_digest_tampering_detected(self):
        manifest, idx = self.complete()
        record = next(iter(idx["records"].values()))
        (self.root / record["artifact_path"]).write_text("tampered", encoding="utf-8")
        self.blocked(manifest, idx, "DIGEST_MISMATCH")

    def test_reordered_event_chain_rejected_even_after_rehash(self):
        manifest, idx = self.complete()
        record = next(iter(idx["records"].values()))
        path = self.root / record["artifact_path"]
        obs = json.loads(path.read_text(encoding="utf-8"))
        obs["observed_steps"][0]["phase"] = "WRONG"
        path.write_text(json.dumps(obs), encoding="utf-8")
        record["artifact_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        self.blocked(manifest, idx, "STEP_0_PHASE_MISMATCH")

    def test_fake_probe_rejected_even_after_rehash(self):
        manifest, idx = self.complete()
        record = next(iter(idx["records"].values()))
        path = self.root / record["artifact_path"]
        obs = json.loads(path.read_text(encoding="utf-8"))
        obs["failure_or_denial_probe"]["source_ref"] = "ci://fake"
        path.write_text(json.dumps(obs), encoding="utf-8")
        record["artifact_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        self.blocked(manifest, idx, "FAILURE_PROBE_MISSING")

    def test_rehashed_steps_from_another_journey_cannot_masquerade_as_correlated(self):
        manifest, idx = self.complete()
        record = next(iter(idx["records"].values()))
        path = self.root / record["artifact_path"]
        obs = json.loads(path.read_text(encoding="utf-8"))
        obs["observed_steps"][1]["correlation_id"] = "other-client-other-booking"
        path.write_text(json.dumps(obs), encoding="utf-8")
        record["artifact_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        self.blocked(manifest, idx, "STEP_1_CORRELATION_MISMATCH")

    def test_business_outcome_and_denial_probes_require_same_correlation(self):
        manifest, idx = self.complete()
        record = next(iter(idx["records"].values()))
        path = self.root / record["artifact_path"]
        obs = json.loads(path.read_text(encoding="utf-8"))
        obs["business_outcome"]["correlation_id"] = "unrelated"
        obs["failure_or_denial_probe"]["correlation_id"] = "unrelated"
        path.write_text(json.dumps(obs), encoding="utf-8")
        record["artifact_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        self.blocked(manifest, idx, "OUTCOME_CORRELATION_MISMATCH")
        self.blocked(manifest, idx, "FAILURE_PROBE_CORRELATION_MISMATCH")

    def test_future_verification_rejected(self):
        manifest, idx = self.complete()
        next(iter(idx["records"].values()))["verified_at"] = "2099-01-01T00:00:00Z"
        self.blocked(manifest, idx, "VERIFICATION_TIME_INVALID")


if __name__ == "__main__":
    unittest.main()
