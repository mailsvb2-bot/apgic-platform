# ADR 0003 — Provider-neutral consultation fulfillment

## Decision

APGIC owns the business meaning of a consultation: who is entitled to join, participant role, allowed join window, lifecycle facts, completion evidence, technical-failure state and the resulting reschedule/refund decision.

A live room is an external capability. Room creation, media transport and provider credentials are executed through a replaceable `COMMUNICATION_PROVIDER` connector. Provider UI presence is evidence only; it is not the canonical consultation state.

## Scoped join authorization

An ALLOW decision requires all of the following:

- canonical Booking is joinable;
- requested identity matches the Booking participant role;
- the latest effective booking-access entitlement is ACTIVE;
- request time is inside a versioned access-policy window;
- selected connector is a routable `COMMUNICATION_PROVIDER`;
- the provider credential is scoped to `ROOM_JOIN` and expires within the configured maximum TTL.

APGIC stores the authorization decision and a provider credential reference, not a raw bearer credential.

## Lifecycle and completion

`READY`, `JOINED`, `LEFT`, `STARTED`, recovery, technical failure and `ENDED` are append-only lifecycle facts with timestamps, provider references, evidence references and idempotency keys.

A Booking cannot become `COMPLETED` because scheduled time elapsed or because a provider UI says the room disappeared. Completion requires an APGIC consultation session with explicit `ENDED` evidence.

## Failure and recovery

Provider/network failure becomes an explicit recoverable or technical state. A versioned recovery policy may retry the current provider, select another certified provider, reschedule, or enter the existing refund path. Communication recovery does not create a second payment attempt and does not manufacture completion evidence.

## Content minimization

Raw audio, video, transcript, recording or avatar media is not APGIC consultation business truth. The canonical fulfillment contract contains only minimal lifecycle and evidence references. Any raw-content retention, if a product later requires it, must be a separate consent/retention capability rather than an implicit field on consultation records.

## Evidence status

Repository tests and PostgreSQL invariants are synthetic domain evidence only. They are not provider production certification, real call-quality evidence, or a claim that external realtime infrastructure is released.
