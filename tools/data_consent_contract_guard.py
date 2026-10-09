#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
PRIVACY = ROOT / "backend/internal/privacy/consent.go"
HTTP = ROOT / "backend/internal/httpapi/demand.go"
CONSENT_HTTP = ROOT / "backend/internal/httpapi/consent.go"
MIGRATION = ROOT / "backend/migrations/000028_r0_consent_ledger.sql"
OPENAPI = ROOT / "contracts/openapi/apgic-v1.yaml"
GENERATED = ROOT / "packages/contracts/src/generated/apgic-v1.ts"


def fail(message: str) -> None:
    print("DATA CONSENT CONTRACT GUARD: FAIL")
    print(f"ERROR: {message}")
    raise SystemExit(1)


def main() -> int:
    privacy = PRIVACY.read_text(encoding="utf-8")
    http = HTTP.read_text(encoding="utf-8")
    consent_http = CONSENT_HTTP.read_text(encoding="utf-8")
    migration = MIGRATION.read_text(encoding="utf-8")
    openapi = OPENAPI.read_text(encoding="utf-8")
    generated = GENERATED.read_text(encoding="utf-8")

    for needle in (
        "PurposeGrowthSessionProjection",
        "PolicyVersion",
        "TextHashOrVersion",
        "GrantedAt",
        "RevokedAt",
        "ProofMetadata",
    ):
        if needle not in privacy:
            fail(f"canonical consent record missing {needle}")

    for needle in (
        "consents.ActiveConsent",
        "privacy.PurposeGrowthSessionProjection",
        "privacy.GrowthConsentScope",
        "policy.Matches(record)",
        "CONSENT_POLICY_UNAVAILABLE",
        "DATA_PURPOSE_CONSENT_REQUIRED",
    ):
        if needle not in http:
            fail(f"growth export is not bound to durable consent: {needle}")
    if "purpose_consent" in http:
        fail("growth export handler still contains client-supplied purpose_consent")

    for needle in (
        "identityAndReferenceFromRequest",
        '"session_ref"',
        '"explicit_consent_grant"',
        '"CLIENT_SESSION_HTTP"',
        "CONSENT_POLICY_VERSION_MISMATCH",
        "policy.PolicyVersion",
        "policy.TextHashOrVersion",
    ):
        if needle not in consent_http:
            fail(f"consent grant proof is not server/session bound: {needle}")

    for needle in (
        "CREATE TABLE consent_records",
        "consent_records_transition_guard",
        "consent_records_delete_guard",
        "consent_records_active_unique",
    ):
        if needle not in migration:
            fail(f"durable consent history invariant missing: {needle}")

    if "operationId: recordConsent" not in openapi or "operationId: revokeConsent" not in openapi:
        fail("OpenAPI does not expose consent lifecycle")
    growth_path = openapi.split("/v1/consultations/{bookingID}/growth-export:", 1)[1].split("/v1/consultations/{bookingID}/complete:", 1)[0]
    if "GrowthExportRequest" in growth_path or "purpose_consent" in growth_path:
        fail("growth-export OpenAPI still trusts a request consent boolean")
    if "export interface GrowthExportRequest" in generated:
        fail("generated client still exposes self-asserted growth consent")
    if '"recordConsent"' not in generated or '"revokeConsent"' not in generated:
        fail("generated client does not include consent lifecycle operations")

    print("DATA CONSENT CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
