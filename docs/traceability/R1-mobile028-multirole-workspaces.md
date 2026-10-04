# R1 evidence — APGIC-MOBILE-028 one-app multi-role workspaces

Requirement: `APGIC-MOBILE-028` — one iOS/Android app, one Identity, authorization-safe role/workspace switching.

Status: **VERIFIED** against exact-candidate CI run `37136033557`, including PostgreSQL tenant-isolation/role-switch proof, installed Android/iOS role switching, Canon conformance, multi-surface guard and final R0 bootstrap gate.

## Canonical execution path

The client does not choose an authorization tenant or role. The signed client session supplies Identity and the server derives available workspaces from durable APGIC truth:

- CLIENT: one workspace for the signed Identity;
- SPECIALIST: available only when that same Identity owns a durable specialist profile;
- ORGANIZATION: one workspace per ACTIVE organization membership in an ACTIVE organization.

`GET /v1/mobile/workspaces` returns only server-derived authorized projections. `GET /v1/mobile/workspaces/{workspaceID}` re-resolves the opaque workspace under the signed Identity. Unknown or foreign workspace IDs return a deterministic DENY without returning the foreign workspace or tenant.

The same production-capable TypeScript client is used by installed-app E2E. It requires CLIENT, SPECIALIST and ORGANIZATION workspaces to retain one `identity_id`, re-resolves each switch through the server, and proves a foreign organization workspace cannot be opened.

## Scoped deep links

Protected client deep links remain short-lived pointers, not authorization grants. A new optional `workspace_id` scope is re-resolved server-side. If supplied for a protected BOOKING/NOTIFICATION, it must be the signed Identity's authorized CLIENT workspace. A same-Identity ORGANIZATION workspace, an unknown workspace, or a foreign workspace fails closed with `DEEPLINK_WORKSPACE_SCOPE_DENY` and no canonical destination.

Clients that do not yet send `workspace_id` retain the existing compatibility path; ownership and tenant authorization are still re-read server-side.

## Automated proof in the candidate

- `backend/internal/httpapi/mobile_workspace_integration_test.go`
  - creates a durable Client Identity;
  - adds Specialist profile without creating a second Identity;
  - creates an Organization membership;
  - lists exactly the authorized role workspaces for that Identity;
  - resolves the owned Organization workspace;
  - creates a second Identity/Organization and proves the first Identity cannot resolve that foreign workspace and receives no tenant disclosure.
- `backend/internal/httpapi/mobile_deeplink_test.go`
  - proves CLIENT workspace can resolve a client-owned protected booking;
  - proves same-Identity wrong-role ORGANIZATION scope and foreign scope both fail closed.
- `apps/mobile/src/mobile-workspace-client.test.ts`
  - validates strict workspace projections and deny behavior;
  - exercises CLIENT → SPECIALIST → ORGANIZATION through the production HTTP client boundary;
  - rejects alternate API origins.
- `tools/mobile_android_capability_e2e.sh` and `tools/mobile_ios_capability_e2e.sh`
  - launch the installed app with the same signed session used by the canonical conformance server;
  - require `workspace-e2e:PASS`;
  - require `workspace-e2e-kinds:CLIENT|SPECIALIST|ORGANIZATION`;
  - require `workspace-e2e-foreign-denied:true`.
- `tools/r1_multisurface_guard.py` and `tools/tests/test_mobile_native_e2e_harness.py`
  - prevent removal of the canonical workspace API/client boundary or either native installed-app proof.

## Candidate verification performed before PR

- all Go packages: PASS;
- mobile Node tests: 76/76 PASS;
- mobile TypeScript typecheck: PASS;
- generated OpenAPI client contract: PASS;
- R1 multi-surface guard: PASS;
- native E2E harness regression guard: PASS.

## WEB scope

APGIC-MOBILE-028 is explicitly the topology requirement for the single installed iOS/Android application. WEB uses the same canonical Identity, authorization and tenant truth but does not have an installed-native one-app lifecycle. The approved compatibility exception `APGIC-MOBILE-028-WEB-NOT-APPLICABLE` records that boundary instead of fabricating WEB runtime evidence.

## Exact CI evidence

- PostgreSQL HTTP role-switch + foreign-tenant denial: run `37136033557`, job `111391596165` — PASS.
- Android installed-app CLIENT → SPECIALIST → ORGANIZATION + foreign denial: job `111391578107` — PASS.
- iOS installed-app role switching: job `111391595435` — PASS.
- Canon / architecture conformance: job `111391579133` — PASS.
- Multi-surface contract guard: job `111391603724` — PASS.
- Final R0 bootstrap gate: job `111392596279` — PASS.

The Requirement Registry now binds these exact run/job references and advances APGIC-MOBILE-028 to `VERIFIED`.
