# APGIC Launch v1 assurance — production decision protocol

Owner: `feat/platform-readiness`. Authority: Canon v7 §§209, 216, 243–245, 305, 307, 309, 314–315, 323, 333–345. This document does not amend the Canon or approve any commercial, fiscal, legal or SLO defaults.

## The exact distinction

1. **Green CI** proves automation ran against a commit. It is not production acceptance.
2. **Staging** proves a near-production runtime, subject to the actual evidence. It is never promoted into production evidence by renaming a file.
3. **Production configuration preflight** proves a signed-off config and the hashes of the release's policies/contracts. It is not itself a production release.
4. **Launch v1 readiness** requires every R0–R4 Requirement ID to be RELEASED with implementation, contract, tests, evidence references and satisfied dependencies; correct freeze metadata and coverage links; real non-synthetic, candidate-bound native/store/payment evidence validated by the existing R4 gate; and production LaunchConfig bound to exact candidate SHA.
5. **Production promotion** is a **separate manual approval and controlled rollout** after the gate. A passing assessment explicitly has `promotion_authorized=false`; no deployment/payout can be inferred from it.

R5+ is not a Launch v1 blocker. Sections with owner-only coverage are flagged for governed analysis, **not automatically classified as missing code**. Frozen coverage/registry disagreement is a hard blocker. All output is machine-readable and human-reviewable.

## Day-to-day command (never makes a release claim)

```bash
python tools/launch_readiness.py \
  --candidate-sha "$(git rev-parse HEAD)" \
  --output evidence/launch-readiness-audit.json
```

This must return an explicit BLOCKED assessment while required real proof is absent. CI publishes the report and summary on every push/PR without suppressing the other gates; it does not fail solely because a product under active development isn't launchable.

## Real R0–R4 production authorization (fail closed)

Run only from the exact reviewed commit on the authorized release workflow with a real approved `config/launch.production.yaml` and the signed-off production R4 evidence manifest. Do not supply CI or staging paths as production proofs.

```bash
python tools/launch_config_guard.py config/launch.production.yaml --mode production --profile R4
python tools/launch_readiness.py \
  --candidate-sha "$(git rev-parse HEAD)" \
  --config config/launch.production.yaml \
  --manifest config/release/r4-approved-production-evidence.yaml \
  --output evidence/launch-readiness-production.json \
  --strict
```

Required source artifacts are inside the repository with locked hashes; the R4 validator verifies evidence index, artifact SHA256, candidate SHA, synthetic flag and required native/store/provider/payment classes. Historical CI-only/placeholder evidence does not pass.

**Operator checks beyond automated acceptance:** independently verify production evidence origin and freshness, provider credentials and legal eligibility without exposing secrets, scheduled backup plus production restore drill with measured RPO/RTO, SLO/alert routing and budget, staging-to-production parity, browser + iOS + Android journeys, canary/rollback, and transaction-safe recovery. Any operator signoff must be immutable, scoped by release SHA and jurisdiction. Don't assert a production launch solely from a preflight artifact.

## Incident and rollback invariants

- Preserve booking/ledger/order/event evidence and append-only audit during rollback. Never undo committed financial facts by rolling back database schema or applying an old backup over active production.
- Freeze monetary operations or affected provider connector with audited, reversible policy/kill switches when reconciliation or tenant isolation is uncertain; preserve read-only access when safe.
- If deploy health passes but business probes (booking, event outbox, checkout callback, entitlement, mobile joins) fail, halt rollout and open an incident, not a success report.
- Production DR requires its **own** isolated restore proof. The repository's staging drill is not a substitute.
- One authorized APGIC server: `92.51.23.254`. No deployment to alternative machines.

## Current baseline

This branch does **not** enable a live launch. The frozen registry contains non-RELEASED R0–R4 requirements, the production index has no records, and no approved R4 production evidence manifest is present. The gate's expected result is `BLOCKED` until real end-to-end functionality and signed-off evidence arrive from all domain branches. Do not artificially set statuses to RELEASED.

## Production evidence ingestion hardening

To populate `canon/evidence/production-evidence-index.yaml`, each production record used by the R4 native/store/payment gate now refers to a JSON observation under `canon/evidence/production/`, conforming to `contracts/jsonschema/production-observation-v1.schema.json`. The observation and index must agree on evidence ID, type, exact 40-character candidate SHA, source URL, verification timestamp and PASS/non-synthetic/PRODUCTION classification; the file must match the indexed SHA256. A generic schema file, CI artifact or staging observation is not acceptable production proof. The external source must be an HTTPS permalink with a meaningful path and a named verifier.

**Security limitation:** a self-authored receipt plus hash is tamper-evident **only relative to Git history**, not a cryptographic attestation from a provider. An operator must independently inspect the HTTPS source's authority, authenticity, content, visibility and timing before declaring production release. No production proof is generated or inserted by CI.

### Critical policy validation is not satisfied by hashes

The `--strict` invocation executes the repository's existing `launch_config_guard.py --mode production --profile R4` itself, before constructing a passing assessment. It checks policy contents, scope, version binding, jurisdiction, SLO/RPO/RTO, retention, allowed providers, real MarketCell thresholds, commerce and multi-provider settlement; merely marking `production_approved: true` and supplying valid SHA256s is insufficient. This is in addition to the separate preflight GitHub Actions step. Missing/invalid policies fail closed without manufacturing production evidence.

## Production journey assurance and no relaxed policy preflight

A production-preflight receipt must now pass the SAME semantic policy checks as the production workflow, not just hash production_approved-looking files. LaunchConfig rules for jurisdiction, retention, SLO, providers, commerce and settlement must fail closed.

Full R4 authorization additionally requires a correlated, non-synthetic, candidate-bound R0–R4 **critical production journey manifest** (launch-critical-journeys-v1) assessed by tools/launch_journey_gate.py. Nine journeys cover tenant denial, outbox recovery, isolated restore, qualified discovery, payment and entitlement, refund reversal, realtime consultation, provider settlement and native-store lifecycle. Every proof must have immutable indexed artifact digest, independent HTTPS source, chronological canonical event chain, end business result and separately observed failure/denial probe. Missing proofs block release; CI and staging cannot create substitute production evidence.

Use --journey-manifest config/release/launch-journeys-approved-production.yaml with --strict for the real release candidate. The provided example intentionally cannot pass. The evidence gate is an admission check, not cryptographic provider attestation or permission to deploy; independently review external proofs and manually authorize a rollout.
