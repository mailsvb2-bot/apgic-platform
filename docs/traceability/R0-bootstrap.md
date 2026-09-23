# R0 bootstrap traceability

This file records implementation locations created by the first APGIC bootstrap. It is evidence inventory, not a status override of the canonical registry.

| Requirement | Initial implementation / evidence |
| --- | --- |
| APGIC-EXEC-001 | canon registry + coverage map + canon_lint.py + CI |
| APGIC-ID-001 | backend/internal/identity + tests |
| APGIC-AUTH-001/002 | backend/internal/authz + negative/step-up tests |
| APGIC-ORG-001/002 | backend/internal/organization + archive semantics + SQL |
| APGIC-CONN-001/002 | backend/internal/connector + idempotent request contract |
| APGIC-EVENT-001 | backend/internal/eventspine + outbox_events schema |
| APGIC-AUDIT-001 | backend/internal/audit + immutable DB trigger |
| APGIC-LEDGER-001 | backend/internal/ledger + immutable DB trigger |
| APGIC-PROD-001 | backend/internal/commerce |
| APGIC-DATA-001 | backend/internal/privacy |
| APGIC-SEC-001 | backend/internal/security + architecture guard |
| APGIC-LEGAL-001 | backend/internal/legal |
| APGIC-CONFIG-008 | backend/internal/launchconfig |
| APGIC-MOBILE-001/002/003/011 | apps/web, apps/mobile, packages/contracts + architecture guard |
| APGIC-PAY-004 | backend/internal/payments + schema + architecture guard |

## Known governance conflict

The supplied registry places APGIC-PAY-004 in R0 while its dependency APGIC-PAY-001 is R2. The bootstrap records this in canon/governance_exceptions.yaml and does not rewrite the supplied Canon. Resolution requires Requirement Change/RFC.
