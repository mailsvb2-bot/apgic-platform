from __future__ import annotations

import hashlib
import hmac
import subprocess
import tempfile
import unittest
from pathlib import Path

from tools import r0_production_admission as gate

SHA = "a" * 40
KEY = b"r0-admission-independent-secret-key-12345"

def registry(status="VERIFIED"):
    return {"requirements": [{"requirement_id":"APGIC-ID-001","release_profile":"R0","status":status},
                             {"requirement_id":"APGIC-BOOK-001","release_profile":"R2","status":"IN_PROGRESS"}]}


class R0ProductionAdmissionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root / "canon/evidence/production").mkdir(parents=True)

    def evidence(self):
        records = {}
        for kind in gate.REQUIRED_PROOFS:
            path = "canon/evidence/production/" + kind + ".json"
            content = ('{"proof":"' + kind + '"}').encode()
            (self.root / path).write_bytes(content)
            digest = hashlib.sha256(content).hexdigest()
            signed = "\\\\n".join((kind, SHA, digest, path, "security-reviewer")).encode()
            records[kind] = {"candidate_sha":SHA,"evidence_class":"PRODUCTION_OPERATIONAL",
                             "synthetic":False,"reviewed":True,"reviewer":"security-reviewer",
                             "path":path,"sha256":digest,
                             "attestation_hmac_sha256":hmac.new(KEY,signed,hashlib.sha256).hexdigest()}
        return {"schema_version":"production-evidence-index-v1","records":records}

    def test_full_exact_candidate_proof_passes_only_with_verified_r0(self):
        self.assertEqual(gate.inspect(registry(), self.evidence(), SHA, self.root, KEY), [])

    def test_staging_probe_shell_is_parseable_and_reuses_canonical_guards(self):
        script = gate.ROOT / "deploy/staging/prove-r0-foundation-readonly.sh"
        subprocess.run(["bash", "-n", str(script)], check=True, capture_output=True)
        code = script.read_text(encoding="utf-8")
        for must in ("assert-authorized-host.sh", "check-staging-runtime.sh",
                     "APGIC_COMMIT_SHA", "APGIC_DATABASE_URL",
                     "apgic-staging-restore-verify.timer",
                     "R0_STAGING_READ_ONLY_PASS"):
            self.assertIn(must, code)
        self.assertNotIn("production_promotion_eligible", code)

    def test_empty_index_fails_all_proofs(self):
        reasons = gate.inspect(registry(), {"schema_version":"production-evidence-index-v1","records":{}},SHA,self.root,KEY)
        for kind in gate.REQUIRED_PROOFS:
            self.assertIn("EVIDENCE_MISSING:"+kind,reasons)

    def test_non_verified_requirement_blocks_even_with_all_live_proofs(self):
        reasons = gate.inspect(registry("IN_PROGRESS"),self.evidence(),SHA,self.root,KEY)
        self.assertTrue(any(x.startswith("CANON_NOT_VERIFIED") for x in reasons))

    def test_no_synthetic_ci_or_cross_candidate_promotion(self):
        for mutate, expected in [
            ({"synthetic":True},"EVIDENCE_NOT_LIVE"),
            ({"evidence_class":"CI_CANDIDATE"},"EVIDENCE_NOT_LIVE"),
            ({"candidate_sha":"b"*40},"EVIDENCE_CANDIDATE_MISMATCH"),
            ({"reviewed":False},"EVIDENCE_NOT_REVIEWED"),
            ({"sha256":"0"*64},"EVIDENCE_DIGEST_MISMATCH"),
            ({"path":"../../etc/passwd"},"EVIDENCE_PATH_OR_DIGEST_INVALID"),
        ]:
            with self.subTest(mutate=mutate):
                doc=self.evidence()
                doc["records"][gate.REQUIRED_PROOFS[0]].update(mutate)
                reasons=gate.inspect(registry(),doc,SHA,self.root,KEY)
                self.assertTrue(any(x.startswith(expected) for x in reasons), reasons)

    def test_missing_attestation_secret_and_mutated_review_are_rejected(self):
        records = self.evidence()
        missing = gate.inspect(registry(), records, SHA, self.root)
        self.assertIn("RELEASE_ATTESTATION_SECRET_UNAVAILABLE", missing)
        records["records"][gate.REQUIRED_PROOFS[0]]["reviewer"] = "attacker"
        blockers = gate.inspect(registry(), records, SHA, self.root, KEY)
        self.assertTrue(any(x.startswith("EVIDENCE_ATTESTATION_INVALID") for x in blockers), blockers)

    def test_unrecognized_schema_and_sha_fail_closed(self):
        self.assertIn("CANDIDATE_SHA_INVALID",gate.inspect(registry(),self.evidence(),"A"*40,self.root,KEY))
        self.assertIn("PRODUCTION_INDEX_SCHEMA_INVALID",gate.inspect(registry(),{"schema_version":"wrong","records":{}},SHA,self.root,KEY))
        self.assertIn("R0_REQUIREMENTS_MISSING",gate.inspect({"requirements":[]},self.evidence(),SHA,self.root,KEY))

if __name__=="__main__":
    unittest.main()
