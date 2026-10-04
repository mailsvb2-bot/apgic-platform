from __future__ import annotations

import unittest

from tools.mobile_release_evidence_guard import validate_binding


def governance(scope: str = "CI_ONLY", approved: bool = False) -> dict:
    return {
        "scope": scope,
        "production_approved": approved,
        "organization": {
            "organization_id": "APGIC_ORG",
        },
        "applications": {
            "ios": {
                "bundle_id": "com.apgic.app",
                "team_id": "TEAM123",
            },
            "android": {
                "application_id": "com.apgic.app",
                "developer_account_id": "PLAY123",
            },
        },
    }


def evidence(candidate_sha: str = "abcdef1234567890", production: bool = False) -> dict:
    prefix = "evidence://" if production else "ci://"
    return {
        "schema_version": "mobile-release-evidence-v1",
        "candidate_sha": candidate_sha,
        "production_release": production,
        "organization_id": "APGIC_ORG",
        "ios": {
            "bundle_id": "com.apgic.app",
            "team_id": "TEAM123",
            "artifact_sha256": "a" * 64,
            "signed": True,
        },
        "android": {
            "application_id": "com.apgic.app",
            "developer_account_id": "PLAY123",
            "artifact_sha256": "b" * 64,
            "signed": True,
        },
        "secret_scan": "PASS",
        "store_account_evidence": prefix + "store-account",
        "signing_audit": prefix + "signing-audit",
    }


class MobileReleaseEvidenceGuardTests(unittest.TestCase):
    def test_ci_evidence_must_bind_exact_candidate_and_governance(self) -> None:
        doc = evidence()
        self.assertEqual(
            validate_binding(doc, governance(), doc["candidate_sha"], "ci"),
            [],
        )

        errors = validate_binding(doc, governance(), "deadbeef1234567", "ci")
        self.assertIn("CANDIDATE_SHA_MISMATCH", errors)

        changed = governance()
        changed["applications"]["ios"]["team_id"] = "OTHER"
        errors = validate_binding(doc, changed, doc["candidate_sha"], "ci")
        self.assertIn("IOS_TEAM_ID_MISMATCH", errors)

    def test_production_is_fail_closed_without_approved_governance_and_external_refs(self) -> None:
        doc = evidence(production=True)
        errors = validate_binding(doc, governance(), doc["candidate_sha"], "production")
        self.assertIn("PRODUCTION_GOVERNANCE_NOT_APPROVED", errors)

        approved = governance(scope="PRODUCTION", approved=True)
        self.assertEqual(
            validate_binding(doc, approved, doc["candidate_sha"], "production"),
            [],
        )

        doc["signing_audit"] = "ci://fake"
        errors = validate_binding(doc, approved, doc["candidate_sha"], "production")
        self.assertIn("SIGNING_AUDIT:PRODUCTION_EVIDENCE_REF_REQUIRED", errors)

    def test_unsigned_or_bad_hash_artifacts_are_rejected(self) -> None:
        doc = evidence()
        doc["ios"]["signed"] = False
        doc["android"]["artifact_sha256"] = "not-a-hash"
        errors = validate_binding(doc, governance(), doc["candidate_sha"], "ci")
        self.assertIn("IOS_SIGNED_REQUIRED", errors)
        self.assertIn("ANDROID_ARTIFACT_SHA256_INVALID", errors)


if __name__ == "__main__":
    unittest.main()
