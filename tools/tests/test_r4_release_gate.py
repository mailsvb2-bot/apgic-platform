from __future__ import annotations

import unittest

from tools import r4_release_gate as gate


def evidence_row(ref: str = "ci://proof") -> dict:
    return {"status": "PASS", "ref": ref}


def valid_ci() -> dict:
    all_types = gate.GATE_REQUIRED["all"]
    return {
        "schema_version": "r4-release-gate-v1",
        "candidate_sha": "abcdef1234567890",
        "synthetic": True,
        "production_candidate": False,
        "evidence": {kind: evidence_row(f"ci://{kind.lower()}") for kind in all_types},
    }


class R4ReleaseGateTests(unittest.TestCase):
    def test_ci_mechanics_accept_complete_synthetic_matrix(self) -> None:
        passed, blockers = gate.evaluate(valid_ci(), "all", "ci")
        self.assertTrue(passed, blockers)

    def test_missing_native_evidence_blocks(self) -> None:
        document = valid_ci()
        del document["evidence"]["REALTIME_E2E"]
        passed, blockers = gate.evaluate(document, "native", "ci")
        self.assertFalse(passed)
        self.assertIn("REALTIME_E2E:MISSING", blockers)

    def test_missing_payment_reconciliation_blocks(self) -> None:
        document = valid_ci()
        del document["evidence"]["LEDGER_RECONCILIATION"]
        passed, blockers = gate.evaluate(document, "payments", "ci")
        self.assertFalse(passed)
        self.assertIn("LEDGER_RECONCILIATION:MISSING", blockers)

    def test_synthetic_evidence_can_never_pass_production(self) -> None:
        document = valid_ci()
        passed, blockers = gate.evaluate(document, "all", "production")
        self.assertFalse(passed)
        self.assertIn("SYNTHETIC_EVIDENCE_FORBIDDEN", blockers)

    def test_production_requires_evidence_scheme(self) -> None:
        document = valid_ci()
        document["synthetic"] = False
        document["production_candidate"] = True
        passed, blockers = gate.evaluate(document, "payments", "production")
        self.assertFalse(passed)
        self.assertTrue(any("PRODUCTION_EVIDENCE_REF_REQUIRED" in item for item in blockers))


if __name__ == "__main__":
    unittest.main()
