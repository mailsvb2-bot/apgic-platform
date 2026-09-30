# APGIC-DATA-001 — growth export privacy boundary

## Requirement

Sensitive/raw consultation/persona content must not flow into growth/intelligence without separate purpose-specific consent.

## Canonical execution path

1. `privacy.CanExportToGrowth` classifies raw consultation/persona data as consent-gated.
2. `demand.Service.RequireBookingOwner` binds a growth export to the booking's client identity before any export decision.
3. `httpapi.trustedClientIdentity` resolves the signed client session and rejects missing/tampered sessions.
4. `POST /v1/consultations/{bookingID}/growth-export` requires both trusted data-subject ownership and purpose consent.
5. Even after consent, `ExportSessionToGrowth` exports only the minimal consultation state projection; `raw_content_included=false`.

## Failure behavior

- Missing trusted session: `CLIENT_SESSION_REQUIRED`.
- Different client identity: `DATA_SUBJECT_IDENTITY_MISMATCH`.
- Missing purpose consent: `DATA_PURPOSE_CONSENT_REQUIRED`.
- No branch returns raw consultation content.

## Automated evidence

- `backend/internal/privacy/classification_test.go`
  - raw consultation/persona classifications require purpose consent.
- `backend/internal/demand/growth_test.go`
  - no-consent export fails;
  - consented export contains no raw content;
  - booking ownership rejects cross-subject access.
- `backend/internal/httpapi/client_session_http_test.go`
  - missing trusted session blocks growth export;
  - a trusted session for a different identity is forbidden before export.
- `.github/workflows/ci.yml`
  - Go format/vet/race suite executes these tests.

## Release status

Keep APGIC-DATA-001 IN_PROGRESS until CI is green on this change and staging/runtime evidence proves the protected HTTP boundary in the deployed release.
