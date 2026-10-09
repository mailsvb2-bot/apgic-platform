# APGIC-DATA-001 — growth export privacy boundary

## Requirement

Sensitive/raw consultation/persona content must not flow into growth/intelligence without separate purpose-specific consent. Consent is canonical evidence, not a request boolean.

## Canonical execution path

1. `privacy.CanExportToGrowth` gates SENSITIVE, RAW_CONSULTATION and RAW_PERSONA and fails closed on unknown classifications.
2. `privacy.ConsentRecord` stores subject, purpose, scope, policy/text version, grant/revoke timestamps, server-owned source and proof metadata.
3. `consent_records` persists consent history in PostgreSQL; evidence fields are immutable, revocation is terminal, hard delete is forbidden.
4. `POST /v1/consents` binds the grant to the trusted client session. The server records session proof metadata instead of trusting client-provided proof claims.
5. `POST /v1/consents/{consentID}/revoke` can revoke only the trusted subject's consent.
6. `demand.Service.RequireBookingOwner` binds growth export to the booking's client identity.
7. `POST /v1/consultations/{bookingID}/growth-export` looks up an active durable consent for purpose `GROWTH_SESSION_PROJECTION` and scope `booking/{bookingID}`.
8. Client-supplied `purpose_consent: true` has no authority and is ignored.
9. Even after valid consent, `ExportSessionToGrowth` exports only the minimal consultation state projection; `raw_content_included=false`.

## Failure behavior

- Missing trusted session: `CLIENT_SESSION_REQUIRED`.
- Different client identity: `DATA_SUBJECT_IDENTITY_MISMATCH`.
- Missing Consent Store: `CONSENT_STORE_UNAVAILABLE`.
- Missing/revoked purpose consent: `DATA_PURPOSE_CONSENT_REQUIRED`.
- Unknown data classification: fail closed.
- No branch returns raw consultation content.

## Automated evidence

- `backend/internal/privacy/classification_test.go`: sensitive/raw consent gate plus unknown-class fail-closed behavior.
- `backend/internal/privacy/consent_test.go`: versioned/scoped consent evidence validation.
- `backend/internal/demand/growth_test.go`: no-consent export fails and consented export contains no raw content.
- `backend/internal/httpapi/consent_test.go`: self-asserted request boolean cannot replace Consent Ledger; grant/revoke is bound to trusted session.
- `backend/internal/runtimepostgres/consent_test.go`: PostgreSQL grant, active lookup, revocation, idempotent replay and retained history.
- `contracts/openapi/apgic-v1.yaml`: consent lifecycle and growth-export contract.
- `.github/workflows/ci.yml`: migration plus PostgreSQL consent lifecycle proof.

## Release status

Keep APGIC-DATA-001 IN_PROGRESS until CI is green on this change and deployed runtime evidence proves the protected HTTP boundary. Production evidence is not fabricated from CI.
