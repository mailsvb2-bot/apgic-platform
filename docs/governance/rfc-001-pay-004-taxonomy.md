# APGIC-RFC-001 — PAY-004 does not depend on payment execution

Status: APPROVED

Reason class: MISSING_DEPENDENCY_BLOCKER

Requirement: APGIC-PAY-004

## Decision

`APGIC-PAY-004` stays in R0. Its dependency on `APGIC-PAY-001` is removed.

R0 owns only the typed split of `PaymentProvider`, `PaymentMethod`, and `PaymentRail`. That split stops contracts from hiding a bank, a method, and a rail inside one `payment_type`. It does not move money.

`APGIC-PAY-001` stays in R2. Payment execution remains a replaceable external provider. APGIC does not accept, store, or transfer client, specialist, or organization funds and does not own a provider monetary balance.

## Why this is not a fintech license

The change does not add acquiring, wallets, stored value, payout accounts, or custody. The frozen v7 FINAL baseline still records the original dependency. The approved delta lives in `canon/requirements/approved-rfcs.yaml`, and `tools/canon_lint.py` accepts only that exact delta.

## Supersedes

`APGIC-GOV-EX-001` is resolved. An open cross-release exception is no longer needed.
