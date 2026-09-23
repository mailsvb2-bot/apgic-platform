# APGIC Platform

APGIC is a standalone marketplace/platform product built from the **v7 FINAL executable canon**.

This repository starts with **R0 — Foundation**. The implementation is intentionally provider-neutral and keeps APGIC's canonical business truth inside APGIC: identity, authorization, organization ownership, products/offers, event spine, audit, ledger/evidence, policies and contracts.

## Canonical stack

- Backend/API/workers/connectors: **Go**
- Web/PWA: **TypeScript + Next.js**
- Native iOS/Android: **TypeScript + React Native**
- Durable business storage: **PostgreSQL**
- Cache/coordination only: **Redis**
- External communications, CRM/growth, persona, payments, calendars, storage/CDN and AI systems: **replaceable providers behind versioned connectors**

## Repository map

- `canon/` — human-readable canon artifact, Requirement Registry, Coverage Map, policy/contract/evidence roots
- `backend/` — APGIC Core and provider-neutral connector boundary
- `contracts/` — public API/event/schema contracts
- `apps/web/` — web/PWA surface
- `apps/mobile/` — React Native iOS/Android surface
- `packages/contracts/` — cross-surface stable types/reason codes (not server business logic)
- `tools/` — conformance/lint tooling
- `.github/workflows/` — CI gates and executable evidence entry points

## Current bootstrap scope

The first implementation commit establishes R0 domain primitives and guardrails:

1. one Identity with multiple roles;
2. tenant/resource authorization and HIGH_RISK step-up decision;
3. universal Organization + archive-first direction lifecycle;
4. provider-neutral Connector Registry boundary;
5. transactional Outbox event envelope;
6. append-only high-risk Audit record contract;
7. non-custodial Ledger entry model;
8. explicit Product/Offer ownership roles;
9. separate `PaymentProvider` / `PaymentMethod` / `PaymentRail` types;
10. PostgreSQL R0 schema baseline;
11. machine-readable canon validation and architecture guard;
12. Web and Native shells consuming the same server contract namespace.

No requirement is marked VERIFIED or RELEASED by this bootstrap. Requirements touched by code are moved only to `IN_PROGRESS`.

## Local checks

```bash
python3 -m pip install -r requirements-dev.txt
python3 tools/canon_lint.py
python3 tools/architecture_guard.py
cd backend && go test ./...
```

Run production work only from approved Launch Configuration. Missing legal, monetary, retention, SLO/RPO/RTO or provider values remain `CONFIG_REQUIRED` and must fail closed.

## Governance

The DOCX canon owns meaning. `canon/requirements/registry.yaml` owns executable traceability/status. Conflicts require a Requirement Change / RFC; implementation never silently rewrites the canon.
