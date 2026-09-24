# ADR 0002 — Strict non-custodial payment control plane

## Decision

APGIC owns Order, normalized Payment evidence, non-custodial Ledger, routing decisions and reconciliation evidence. Actual monetary execution remains outside APGIC in a certified `PAYMENT_PROVIDER` connector.

`PaymentProvider`, `PaymentMethod` and `PaymentRail` remain separate canonical concepts. A routing decision captures a versioned policy, selected provider configuration, method, rail, jurisdiction, candidate exclusions and provider-health snapshot before an irreversible attempt.

## Non-custodial boundary

APGIC Core must not introduce a user/specialist monetary wallet, custodial balance, card acquiring, tokenization/PCI, 3DS, bank antifraud, KYC/KYB/AML, FX, chargeback execution or payout rail. Provider configuration requires `execution_owner=EXTERNAL_PROVIDER`; payment instructions contain only scoped execution intent and immutable Order economics.

Provider callbacks are normalized into minimal evidence. Raw provider payloads are not canonical business truth; the durable receipt stores a payload digest and provider event identity. The `(provider_instance_id, provider_event_id)` identity is unique.

## Safe fallback

Cross-provider fallback is allowed only before an irreversible send or after a proven terminal failure. `SENT`, `PENDING` and `AMBIGUOUS` attempts block creation of another attempt for the same Order. An ambiguous attempt can become terminal only with provider evidence; it cannot silently fan out to another provider.

## Versioning and history

Provider configuration and routing decisions are append-only snapshots. Historical Order/Payment/Ledger truth is never rewritten when provider priority, health or availability changes. Multiple certified provider instances may be active at the same time.

## Evidence status

The repository CI proves domain rules and PostgreSQL invariants with synthetic providers only. It is not production PSP certification, store evidence, settlement proof or a claim that any real provider is approved.
