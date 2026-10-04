# APGIC-MOBILE-030 — Store production release Definition of Done

Status: **IN_PROGRESS**.

## Gap closed in this change

The R4 store production gate previously validated evidence rows but did not validate the Canon dependency state of APGIC-MOBILE-030 itself. A complete-looking production evidence manifest could therefore pass the store evidence matrix even while launch-critical prerequisite requirements remained IN_PROGRESS.

Production store/all evaluation now fails closed unless every APGIC-MOBILE-030 dependency in the canonical Requirement Registry is present and has status `VERIFIED`.

The dependency check is derived from `canon/requirements/registry.yaml`; no second hardcoded dependency list is introduced.

## Current Canon blockers

At this point APGIC-MOBILE-030 cannot be VERIFIED because these direct dependencies remain IN_PROGRESS:

- APGIC-MOBILE-016 — organization-owned store identities and governed release lifecycle
- APGIC-MOBILE-017 — versioned StorePolicySnapshot and commerce decision matrix
- APGIC-MOBILE-018 — server-verified store purchase entitlement bridge
- APGIC-MOBILE-019 — store renewal/refund/revocation reconciliation
- APGIC-MOBILE-020 — cross-platform account deletion contract
- APGIC-MOBILE-022 — MobileDependencyRegistry and privacy supply-chain gate
- APGIC-MOBILE-023 — mobile privacy declarations match actual build
- APGIC-MOBILE-025 — mobile observability and performance SLO gate

APGIC-MOBILE-027 and APGIC-MOBILE-029 are VERIFIED.

## Safety property

For `--mode production --gate store` and `--mode production --gate all`:

1. evidence manifest validation still applies;
2. synthetic evidence remains forbidden;
3. production candidate classification remains mandatory;
4. production evidence refs remain mandatory;
5. Canon dependencies are additionally mandatory and must all be VERIFIED.

Missing dependency rows and non-VERIFIED dependency states are fail-closed blockers.

## Automated proof

`tools/tests/test_r4_release_gate.py` covers:
- IN_PROGRESS dependency blocks store production readiness;
- switching that dependency to VERIFIED removes the blocker;
- a dependency named by MOBILE-030 but absent from the registry fails closed.

MOBILE-030 remains **IN_PROGRESS** until its required dependent requirements and real production/store evidence are complete.
