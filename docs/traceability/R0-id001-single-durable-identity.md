# APGIC-ID-001 — single durable identity across roles

## Requirement

A person keeps one canonical Identity while gaining additional profiles/roles. Role transitions must not create independent account truth.

## Proven path

1. A trusted client session creates a HelpIntent.
2. PostgreSQL `journeyStore.CreateIntent` persists one `identities` row and the `CLIENT` role for that session Identity.
3. The same signed session cookie calls `PUT /v1/specialist/profile`.
4. PostgreSQL `UpsertProfile` reuses that same Identity and adds `SPECIALIST` idempotently.
5. The proof verifies:
   - client and specialist surfaces expose the same `identity_id`;
   - durable roles are exactly `CLIENT` and `SPECIALIST`;
   - identity version advances to 2;
   - exactly one row exists in `identities`;
   - exactly one specialist profile references that same Identity.

## Automated evidence

- `backend/internal/runtimepostgres/identity_roles_test.go`
- `backend/internal/httpapi/identity_multirole_integration_test.go`
- `apps/web/e2e/foundation.spec.ts`
- `.github/workflows/ci.yml`

## Release status

This satisfies DOMAIN_OR_CONTRACT_TEST plus an HTTP/PostgreSQL integration proof for the client-to-specialist role transition. Keep the requirement IN_PROGRESS until the project release-status policy promotes the evidence set; do not infer production deployment from CI alone.
