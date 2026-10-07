# R0 real WEB Organization/Product lifecycle evidence

This slice replaces the mocked-only browser confidence gap with a real cross-boundary proof:

`WEB browser -> Next.js /v1 proxy -> APGIC HTTP API -> PostgreSQL -> browser reload`.

## Covered user journey

`apps/web/e2e/r0-org-product-postgres.spec.ts` runs only in the dedicated PostgreSQL-backed CI gate and does not intercept or mock the APGIC API.

The browser:

1. opens the real Organization workspace;
2. creates an Organization through `POST /v1/organizations`, receiving the canonical signed Identity session;
3. creates a generic `SERVICE` direction through the real API;
4. reads the canonical Organization back from PostgreSQL-backed API state and resolves the generated direction ID;
5. creates a Product draft with explicit commercial owner, author and revenue-beneficiary references;
6. reads the Product back from the canonical list and checks every explicit ownership/revenue field;
7. publishes the Product through the real publication endpoint;
8. archives the direction through the real archive endpoint;
9. proves the published Product remains readable after direction archival;
10. reloads the browser and proves Organization, archived Direction and published Product state survive the full round trip.

## Requirement coverage

### APGIC-ORG-001

The proof exercises the actual user-visible Organization -> Direction path through the signed session, canonical HTTP owner and PostgreSQL runtime. It demonstrates that a generic non-education-specific direction belongs to the created Organization.

### APGIC-PROD-001

The proof exercises Product create -> read -> publish -> reload with explicit:

- `owner_type=ORGANIZATION`;
- `owner_id`;
- `commercial_owner_ref`;
- `author_refs`;
- `revenue_beneficiary_ref`;
- `organization_direction_id`.

No role is reconstructed from one implicit owner field.

### APGIC-ORG-002

The browser proof demonstrates the visible archive path and confirms that the published Product remains readable after archival. The deeper Booking/Order/Ledger/Audit preservation proof remains owned by `backend/internal/runtimepostgres/journey_test.go` and `docs/traceability/R0-org002-history-links.md`.

## CI evidence

`.github/workflows/ci.yml` runs the real browser journey in the same PostgreSQL-backed gate that boots the APGIC API in `STAGING` mode. The test writes:

`evidence/r0-org-product-web-e2e.json`

and CI uploads it as:

`r0-org-product-web-e2e-evidence`.

The artifact is bound to `APGIC_CANDIDATE_SHA` and explicitly records `production_evidence=false`.

## Exact candidate evidence

The first exact candidate that carries this proof is:

- candidate: `bc2ac5521d818c1a4fb2b5ffd8efff41fb44611f`;
- CI run: `37582060969` — SUCCESS;
- PostgreSQL migration / invariant proof job: `112663755399` — SUCCESS;
- Playwright result inside that job: `2 passed`, including `WEB organization product lifecycle persists through real API and PostgreSQL`;
- artifact: `r0-org-product-web-e2e-evidence`;
- artifact ID: `11465255409`;
- artifact digest: `sha256:890718a7b1f09df2f7d3ff418faab6f671a376ca22ffc8fb2980df88ab721578`;
- full R0 bootstrap job: `112667182228` — SUCCESS.

This evidence is exact CI evidence for the candidate. It is not production deployment evidence.

## Status discipline

This document does not by itself change Registry status. Exact run/job/artifact identifiers must be bound after a concrete candidate passes the gate. Requirements that explicitly demand separate staging/release evidence remain `IN_PROGRESS` until that evidence exists.


## Deployed staging proof

On candidate `0df3a19bbcc67208b5f67ab07c57b40fa28c4470`, the public staging deployment at `https://apgic.ru` was exercised outside CI through the canonical HTTP runtime:

1. create Organization;
2. create a generic `SERVICE` direction;
3. create a Product draft with explicit commercial owner, author and revenue beneficiary;
4. publish the Product;
5. archive the direction;
6. read Organization and Product state back after the archive.

The readback showed the direction as `ARCHIVED` while the Product remained `PUBLISHED` with the same explicit ownership/revenue refs.

Evidence: `canon/evidence/staging-org-product-20261007T125100Z.json`.

This closes the previously documented missing staging/runtime proof.

## Native installed-app surface proof

Candidate `bc5f9b0e657a2d64c5090d4d89cb1a6ac5522c60` adds the same canonical Organization/Product journey to the installed native app and runs it against the deployed `https://apgic.ru` runtime:

- iOS Simulator job `112816037630` in run `37628404273` — SUCCESS;
- Android emulator job `112816038097` in run `37628404273` — SUCCESS;
- iOS artifact `ios-native-simulator-build` digest `sha256:ddb456a354c7d45becf039aa703f7f29993c9af1e562e0b1b3b2e06777996094`;
- Android artifact `android-native-debug-build` digest `sha256:63ffb145e81bf25d5acce46bafcb9e564b163e146f555344ecccde81a77d4530`.

The native client owns no parallel Organization/Product business truth. It calls the canonical HTTP owners, and the installed-app proof requires create -> publish -> archive -> readback with explicit ownership/revenue fields preserved.

With WEB, deployed staging, IOS and ANDROID evidence now attached, APGIC-ORG-001, APGIC-ORG-002 and APGIC-PROD-001 are promoted to `VERIFIED`.
