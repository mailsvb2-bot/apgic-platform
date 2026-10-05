# APGIC-DEMAND-001 — HelpIntent confirmation across web and native

Status: **VERIFIED**.

## Canon requirement

Free-form demand is interpreted into a HelpIntent that the user can inspect and correct before confirmation. The interpretation must not claim a diagnosis.

Required evidence:
- DOMAIN_OR_CONTRACT_TEST
- E2E_OR_STAGING_PROOF

Because R1 is a launch profile, VERIFIED also requires real WEB/iOS/Android surface proof under the multi-surface guard.

## Canonical server truth

All clients use the same server endpoints and HelpIntent entity:

- `POST /v1/help-intents`
- `POST /v1/help-intents/{id}/confirm`

The backend:
- derives topics/goals/context from a versioned deterministic interpretation lexicon;
- always returns `diagnosis_asserted=false`;
- returns an explicit notice that interpretation is not a diagnosis;
- allows the user to submit corrected topics/goals/context;
- preserves the same canonical client Identity from draft to confirmation.

## Web proof

`apps/web/e2e/foundation.spec.ts` proves the browser user can:
- enter free text;
- see the non-diagnosis notice;
- modify suggested topics;
- confirm the corrected HelpIntent before matching.

Existing staging runtime evidence explicitly lists APGIC-DEMAND-001 as supported and records live Playwright execution against `apgic.ru`.

## Native implementation

`apps/mobile/src/mobile-demand-client.ts` consumes the generated API contract and the same canonical endpoints.

The production React Native UI now exposes:
- free-text demand input;
- server interpretation;
- the non-diagnosis notice;
- editable suggested-topic selection;
- explicit user confirmation.

No native-owned HelpIntent state machine or parallel business truth is introduced.

## Native installed-app proof

Android and iOS capability suites launch the actual built applications against the isolated canonical backend.

Android now proves the production user journey itself rather than relying on a hidden test hook:
1. opens the visible “С чем нужна помощь” field;
2. enters `anxiety sleep`;
3. runs the production interpretation action;
4. observes both suggested topics and `diagnosis_asserted=false`;
5. deselects `anxiety`;
6. invokes the production confirmation action;
7. proves the confirmed surface contains `sleep` and no longer contains `anxiety`.

iOS retains the installed-app contract probe and must independently prove the same canonical HelpIntent/Identity and no-diagnosis invariants.

## Exact verification evidence

Fresh CI run `37337781585` proved the complete required chain on candidate `509ec0578d5b6c6cee602e713d44d82d869fe26d`:

- Web production build + browser E2E: job `111857429352`;
- Android installed-app HelpIntent correction/no-diagnosis flow: job `111857429380`;
- iOS installed-app HelpIntent/Identity/no-diagnosis proof: job `111857429442`;
- Go/domain verification: job `111857429423`;
- PostgreSQL migration/invariant proof: job `111857429283`;
- multi-surface contract guard: job `111857429542`;
- Canon/architecture conformance: job `111857429368`.

These exact refs are bound in the Requirement Registry. APGIC-DEMAND-001 is therefore promoted to VERIFIED on this evidence-bearing head. Any subsequent change to the requirement implementation, contract, tests, or evidence mapping requires a fresh verification run before merge/release.
