from __future__ import annotations

import copy
import unittest

from tools.release_evidence import (
    build_requirement_evidence,
    load_ci_evidence_map,
    load_registry_requirements,
)


ALL_GREEN_GATES = {
    "canon": "success",
    "backend": "success",
    "postgres-invariants": "success",
    "restore-drill": "success",
    "web-build": "success",
    "mobile-typecheck": "success",
    "android-native-build": "success",
    "ios-native-build": "success",
    "client-contract-presence": "success",
}


class ReleaseEvidenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.registry = load_registry_requirements()
        self.evidence_map = load_ci_evidence_map()

    def test_requirement_evidence_distinguishes_ci_proven_partial_and_unproven(self) -> None:
        result = build_requirement_evidence(
            ALL_GREEN_GATES,
            registry=self.registry,
            evidence_map=self.evidence_map,
        )

        self.assertEqual(result["APGIC-DATA-001"]["status"], "CI_PROVEN")
        self.assertEqual(result["APGIC-MOBILE-002"]["status"], "CI_PROVEN")
        self.assertEqual(result["APGIC-CONFIG-005"]["status"], "PARTIAL")
        self.assertEqual(
            result["APGIC-CONFIG-005"]["unproven_evidence"],
            ["RELEASE_EVIDENCE"],
        )
        self.assertEqual(result["APGIC-DR-001"]["status"], "UNPROVEN")
        self.assertEqual(
            result["APGIC-DR-001"]["unproven_evidence"],
            ["RESTORE_DR_PROOF"],
        )
        self.assertEqual(result["APGIC-MOBILE-012"]["status"], "PARTIAL")
        self.assertNotIn(
            "SECRET_SCAN",
            result["APGIC-MOBILE-012"]["unproven_evidence"],
        )
        self.assertEqual(
            result["APGIC-MOBILE-012"]["unproven_evidence"],
            ["ANDROID_BUILD_PROOF", "IOS_BUILD_PROOF", "RELEASE_EVIDENCE"],
        )

    def test_mobile005_native_e2e_surface_refs_are_ci_proven(self) -> None:
        result = build_requirement_evidence(
            ALL_GREEN_GATES,
            registry=self.registry,
            evidence_map=self.evidence_map,
        )
        self.assertEqual(result["APGIC-MOBILE-005"]["status"], "CI_PROVEN")
        native = result["APGIC-MOBILE-005"]["proven_evidence"]["NATIVE_E2E"]
        self.assertEqual(
            native["gates"],
            ["android-native-build", "ios-native-build"],
        )
        self.assertTrue(
            any(ref.startswith("surface://IOS/") for ref in native["proof_refs"])
        )
        self.assertTrue(
            any(ref.startswith("surface://ANDROID/") for ref in native["proof_refs"])
        )

    def test_non_green_gate_cannot_back_a_claim(self) -> None:
        gates = dict(ALL_GREEN_GATES)
        gates["backend"] = "failure"

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                gates,
                registry=self.registry,
                evidence_map=self.evidence_map,
            )

    def test_explicit_evidence_ref_is_valid_traceability_for_staging_proof(self) -> None:
        result = build_requirement_evidence(
            ALL_GREEN_GATES,
            registry=self.registry,
            evidence_map=self.evidence_map,
        )
        self.assertIn(
            "E2E_OR_STAGING_PROOF",
            result["APGIC-EXEC-001"]["proven_evidence"],
        )

    def test_proof_ref_must_belong_to_requirement_traceability(self) -> None:
        evidence_map = copy.deepcopy(self.evidence_map)
        evidence_map["requirements"]["APGIC-ID-001"]["claims"][
            "DOMAIN_OR_CONTRACT_TEST"
        ]["proof_refs"].append("tools/release_evidence.py")

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                ALL_GREEN_GATES,
                registry=self.registry,
                evidence_map=evidence_map,
            )

    def test_malformed_surface_ref_is_rejected(self) -> None:
        registry = copy.deepcopy(self.registry)
        evidence_map = copy.deepcopy(self.evidence_map)
        bad_ref = "surface://IOS/../../forged"
        registry["APGIC-MOBILE-005"]["evidence_refs"].append(bad_ref)
        evidence_map["requirements"]["APGIC-MOBILE-005"]["claims"]["NATIVE_E2E"][
            "proof_refs"
        ].append(bad_ref)

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                ALL_GREEN_GATES,
                registry=registry,
                evidence_map=evidence_map,
            )

    def test_every_required_evidence_kind_must_be_classified(self) -> None:
        evidence_map = copy.deepcopy(self.evidence_map)
        evidence_map["requirements"]["APGIC-A11Y-001"]["unproven_evidence"] = []

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                ALL_GREEN_GATES,
                registry=self.registry,
                evidence_map=evidence_map,
            )

    def test_every_r0_requirement_must_be_mapped(self) -> None:
        evidence_map = copy.deepcopy(self.evidence_map)
        evidence_map["requirements"].pop("APGIC-EXEC-001")

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                ALL_GREEN_GATES,
                registry=self.registry,
                evidence_map=evidence_map,
            )

    def test_unknown_requirement_is_rejected(self) -> None:
        evidence_map = copy.deepcopy(self.evidence_map)
        evidence_map["requirements"]["APGIC-NOT-REAL"] = {
            "claims": {},
            "unproven_evidence": [],
        }

        with self.assertRaises(SystemExit):
            build_requirement_evidence(
                ALL_GREEN_GATES,
                registry=self.registry,
                evidence_map=evidence_map,
            )


if __name__ == "__main__":
    unittest.main()
