# ADR-0001 — Canonical truth, monorepo and provider boundaries

Status: Accepted for R0 bootstrap

## Decision

APGIC is implemented as one repository containing server, web, native, contracts, canon artifacts and conformance tooling.

APGIC Core owns business truth. External providers own only their execution/runtime state. Provider SDKs are allowed only inside connector/adapter boundaries and cannot own APGIC canonical entities.

Web/PWA/iOS/Android consume one versioned backend contract. Shared client code is limited to contracts, enums/reason codes, validation, localization/design tokens and analytics event schemas; server authorization, booking/payment/ledger state machines remain server-owned.

## Donor repositories

BusinessAIOS, ClientPlatform, Universal-Communication-Runtime and Virtual-Persona-Runtime are read-only donors. Reuse follows:

`study -> provenance snapshot -> copy outside donor workspace -> APGIC-native adaptation -> tests`

No APGIC task may mutate donor repositories.

## Why

This preserves Structural Rigidity: one owner, one name/ID, explicit state/error semantics, no duplicate business truth and no hidden dependency on a branded provider.
