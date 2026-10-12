# ADR 0004 — R3 scoped room issuer / fail-closed provider boundary

- `APGIC_COMM_ISSUER_URL`: trusted HTTPS provider-neutral room issuance endpoint (versioned `apgic.room-join.v1` POST), `APGIC_COMM_ISSUER_TOKEN`: service credential, `APGIC_COMM_JOIN_ORIGIN`: exact allowed HTTPS room origin.
- `APGIC_COMM_JOIN_PROVIDER_INSTANCE_ID`: APGIC-owned active COMMUNICATION_PROVIDER connector UUID, `APGIC_COMM_JOIN_POLICY_VERSION`: explicit persisted version of `communication_access_policy_versions`.
- APGIC validates authenticated participant, canonical confirmed booking, **latest active** booking-access entitlement, booked time/policy and configured provider. Missing any prerequisite blocks the join. Issuer calls are scoped `ROOM_JOIN`, short-lived, idempotency keyed; HTTP redirects are blocked.
- Successful provider issuance is still not call completion. APGIC atomically stores `communication_join_authorizations` referencing the provider's opaque credential reference. No room URI, bearer token, transcript or media key is stored. After DB guard failure, the grant is withheld from client; no payment is charged.
- Signed `JOINED/STARTED/ENDED` provider events continue to own authoritative attendance and completion. Production proof requires a configured certified provider, issuer interoperability and real two-party call (WEB + IOS + ANDROID) with reconnect and disaster recovery. A synthetic test is insufficient.
- The connector contract supports any external media service implementing the scoped contract; APGIC does not import UCR internals or share a donor's DB.

Issuer response:
```json
{"provider_credential_ref":"opaque/non-secret/reference", "join_uri":"https://media.example/room?token=short-lived", "expires_at":"2026-10-12T12:35:00Z"}
```
