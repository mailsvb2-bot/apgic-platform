# APGIC-MOBILE-020 — Cross-platform account deletion

Status: **IN_PROGRESS**.

## Canon requirement

A user who can create an APGIC account must be able to initiate deletion from native iOS/Android and from authenticated web. All surfaces converge on the same canonical DeleteAccountRequest lifecycle; deactivation must never be presented as deletion.

Required evidence:
- IOS_DELETION_E2E
- ANDROID_DELETION_E2E
- WEB_DELETION_E2E
- RETENTION_TEST

## Canonical path

All surfaces use the same backend endpoint:

`POST /v1/account-deletions`

The backend:
1. resolves the trusted client Identity from the signed session;
2. rejects caller-supplied foreign Identity values;
3. creates one canonical DeleteAccountRequest;
4. classifies profile/help-intent data for erasure;
5. retains financial evidence only with an explicit reason;
6. records provider erasure evidence;
7. finishes as `PARTIALLY_RETAINED_WITH_REASON` when required financial evidence remains;
8. returns the same deletion id/state on retry with `idempotent=true`.

## Web proof

`apps/web/e2e/foundation.spec.ts` exercises the real browser journey through account deletion and asserts:
- `PARTIALLY_RETAINED_WITH_REASON`;
- deactivation is false;
- accounting evidence is retained;
- APGIC does not delete the ledger;
- replay is idempotent.

## Installed-app proof

The Android and iOS native capability suites now launch the actual built apps with a signed CI client session and execute the canonical deletion endpoint through `runNativeDeletionE2E`.

Both installed-app probes require:
- `deletion-e2e:PASS`;
- state `PARTIALLY_RETAINED_WITH_REASON`;
- `deactivation=false`;
- profile erased;
- ledger retained;
- second submission idempotent.

The native client rejects a response that masquerades deactivation as deletion or changes canonical state on replay.

## Retention proof

Deletion state-machine/domain tests, R1 PostgreSQL invariants, and the retention contract guard prove the retention classification and fail-closed retention matrix mechanics.

## Remaining dependency blocker

APGIC-MOBILE-020 deliberately remains **IN_PROGRESS** even when its own evidence map reaches `unproven_evidence: []`, because dependency `APGIC-CONFIG-004` still lacks production `RELEASE_EVIDENCE`.

CI-only retention values are not production retention policy and must not be promoted as such.
