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


def valid_production_index(document: dict, gate_name: str) -> dict:
    records = {}
    for evidence_type in gate.GATE_REQUIRED[gate_name]:
        evidence_id = evidence_type.lower()
        document["evidence"][evidence_type] = evidence_row(f"evidence://{evidence_id}")
        records[evidence_id] = {
            "evidence_type": evidence_type,
            "status": "PASS",
            "synthetic": False,
            "candidate_sha": document["candidate_sha"],
            "source_ref": f"github-actions://run/{evidence_id}",
            "artifact_sha256": "a" * 64,
            "verified_at": "2026-10-05T17:42:48Z",
        }
    return {"schema_version": "production-evidence-index-v1", "records": records}


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


    def test_native_production_dependencies_must_be_verified(self) -> None:
        registry = {
            "requirements": [
                {
                    "requirement_id": "APGIC-MOBILE-015",
                    "dependencies": ["APGIC-MOBILE-003", "APGIC-MOBILE-012"],
                },
                {
                    "requirement_id": "APGIC-MOBILE-003",
                    "status": "VERIFIED",
                    "dependencies": [],
                },
                {
                    "requirement_id": "APGIC-MOBILE-012",
                    "status": "IN_PROGRESS",
                    "dependencies": [],
                },
            ]
        }
        self.assertEqual(
            gate.canon_dependency_blockers(registry, "APGIC-MOBILE-015"),
            ["APGIC-MOBILE-012:CANON_STATUS_IN_PROGRESS"],
        )

        registry["requirements"][2]["status"] = "VERIFIED"
        self.assertEqual(
            gate.canon_dependency_blockers(registry, "APGIC-MOBILE-015"),
            [],
        )

    def test_store_production_dependencies_must_be_verified(self) -> None:
        registry = {
            "requirements": [
                {
                    "requirement_id": "APGIC-MOBILE-030",
                    "dependencies": ["APGIC-MOBILE-016", "APGIC-MOBILE-029"],
                },
                {
                    "requirement_id": "APGIC-MOBILE-016",
                    "status": "IN_PROGRESS",
                    "dependencies": [],
                },
                {
                    "requirement_id": "APGIC-MOBILE-029",
                    "status": "VERIFIED",
                    "dependencies": [],
                },
            ]
        }
        self.assertEqual(
            gate.canon_dependency_blockers(registry, "APGIC-MOBILE-030"),
            ["APGIC-MOBILE-016:CANON_STATUS_IN_PROGRESS"],
        )

        registry["requirements"][1]["status"] = "VERIFIED"
        self.assertEqual(
            gate.canon_dependency_blockers(registry, "APGIC-MOBILE-030"),
            [],
        )

    def test_missing_store_dependency_is_fail_closed(self) -> None:
        registry = {
            "requirements": [
                {
                    "requirement_id": "APGIC-MOBILE-030",
                    "dependencies": ["APGIC-MOBILE-016"],
                }
            ]
        }
        self.assertEqual(
            gate.canon_dependency_blockers(registry, "APGIC-MOBILE-030"),
            ["APGIC-MOBILE-016:CANON_DEPENDENCY_MISSING"],
        )

    def test_synthetic_evidence_can_never_pass_production(self) -> None:
        document = valid_ci()
        passed, blockers = gate.evaluate(document, "all", "production")
        self.assertFalse(passed)
        self.assertIn("SYNTHETIC_EVIDENCE_FORBIDDEN", blockers)

    def test_production_requires_evidence_scheme(self) -> None:
        document = valid_ci()
        document["synthetic"] = False
        document["production_candidate"] = True
        passed, blockers = gate.evaluate(document, "payments", "production", {"schema_version": "production-evidence-index-v1", "records": {}})
        self.assertFalse(passed)
        self.assertTrue(any("PRODUCTION_EVIDENCE_REF_REQUIRED" in item for item in blockers))

    def test_fabricated_production_evidence_uri_is_rejected(self) -> None:
        document = valid_ci()
        document["synthetic"] = False
        document["production_candidate"] = True
        for evidence_type in gate.GATE_REQUIRED["payments"]:
            document["evidence"][evidence_type] = evidence_row(f"evidence://{evidence_type.lower()}")
        passed, blockers = gate.evaluate(
            document,
            "payments",
            "production",
            {"schema_version": "production-evidence-index-v1", "records": {}},
        )
        self.assertFalse(passed)
        self.assertTrue(any("PRODUCTION_EVIDENCE_NOT_FOUND" in item for item in blockers))

    def test_indexed_immutable_production_evidence_can_pass_matrix(self) -> None:
        document = valid_ci()
        document["synthetic"] = False
        document["production_candidate"] = True
        production_index = valid_production_index(document, "payments")
        passed, blockers = gate.evaluate(document, "payments", "production", production_index)
        self.assertTrue(passed, blockers)

    def test_production_evidence_is_bound_to_candidate_sha(self) -> None:
        document = valid_ci()
        document["synthetic"] = False
        document["production_candidate"] = True
        production_index = valid_production_index(document, "payments")
        first = next(iter(production_index["records"].values()))
        first["candidate_sha"] = "deadbeef"
        passed, blockers = gate.evaluate(document, "payments", "production", production_index)
        self.assertFalse(passed)
        self.assertIn(
            f"{first['evidence_type']}:PRODUCTION_EVIDENCE_CANDIDATE_MISMATCH",
            blockers,
        )


if __name__ == "__main__":
    unittest.main()
