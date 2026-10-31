# Replaceable CommunicationProvider — LiveKit reference implementation

This is a **provider-side adapter**, not a second APGIC booking, payment or consultation lifecycle owner. The APGIC core remains independent of LiveKit. A different provider can implement the same `apgic.room-join.v1` contract.

## Real media flow

1. APGIC checks CONFIRMED booking, exact client/specialist identity, entitlement version and time policy, then calls private `POST /v1/room-grants` with its bearer service credential.
2. The adapter issues two **distinct** LiveKit JWTs with room-join/publish/subscribe grants and expiry not exceeding the APGIC authorization. Only the opaque credential reference is persisted in APGIC. The `join_uri` carries the short-lived token in a URL **fragment**, not query parameters.
3. Each participant opens the isolated HTTPS room UI. The bundled official `livekit-client` JavaScript SDK connects to the configured WSS LiveKit SFU and attempts real camera+microphone publication. The UI handles reconnection, network loss, mute/camera changes and voluntary leave without minting APGIC lifecycle facts.
4. LiveKit server events arrive on `POST /v1/livekit-webhooks`. The adapter verifies **LiveKit's signed JWT plus SHA-256 hash of the raw body** before inspecting room or participant identities. Only identities issued by this room adapter are mapped to business roles.
5. Authenticated LiveKit `participant_joined` events become JOINED, and STARTED requires **audio track publication from both** client and specialist. A dropout produces LEFT and RECOVERY_STARTED; restored two-party media produces RECOVERY_SUCCEEDED.
6. Each participant explicitly clicks **Завершить консультацию**. The adapter stores the independently authenticated finish intents, then waits for the actual LiveKit `room_finished` webhook. ENDED is emitted only when both finish intents, provider room closure, media evidence and a minimum 60-second overlap are all present. Otherwise, an active session produces TECHNICAL_FAILURE — **never a fictitious COMPLETED**.
7. Signed Ed25519 events are persisted in an append-only local JSON outbox (fsync + atomic rename) with monotonically increasing per-booking sequence. It retries APGIC until APPLY or DUPLICATE is returned. APGIC's existing signed-provider ingestion and PostgreSQL lifecycle remain canonical, including after an API restart. This outbox contains **metadata only**; no transcript, media or diagnoses.
8. Reschedule/refund after terminal technical failure is **not an automatic charge/refund**; external payment and booking owners must fulfill those workflows. Never charge twice merely because a media connection failed.

## Required deployment configuration

The adapter runs behind a dedicated HTTPS reverse proxy. Do **not** expose the service's plaintext HTTP port on the public Internet. Configure (all secrets through a secret manager, never Git):

| Environment | Description |
|---|---|
| `APGIC_COMM_ISSUER_TOKEN` | Secret shared only by APGIC and adapter (24+ bytes) |
| `APGIC_COMM_JOIN_ORIGIN` | Adapter HTTPS public origin, exactly matching APGIC's allowed join origin |
| `APGIC_COMM_JOIN_PROVIDER_INSTANCE_ID` | Existing active APGIC `COMMUNICATION_PROVIDER` connector UUID |
| `APGIC_COMM_REF_APGIC_WEBHOOK_URL` | HTTPS API URL ending `/v1/consultation-provider-webhooks` |
| `APGIC_COMM_REF_ED25519_PRIVATE_KEY_B64` | Base64 Ed25519 64-byte private signing key |
| `APGIC_COMM_REF_KEY_ID` | Ed25519 provider signing key ID |
| `LIVEKIT_API_KEY` | LiveKit API key |
| `LIVEKIT_API_SECRET` | LiveKit API secret (32+ chars) |
| `LIVEKIT_URL` | WSS URL of configured LiveKit Cloud/self-hosted SFU |
| `APGIC_COMM_REF_STATE_DIR` | Persistent volume with restricted permissions, backed up before upgrades |

Configure APGIC separately: `APGIC_COMM_ISSUER_URL=https://<provider-origin>/v1/room-grants`, `APGIC_COMM_ISSUER_TOKEN`, `APGIC_COMM_JOIN_ORIGIN`, `APGIC_COMM_JOIN_PROVIDER_INSTANCE_ID`, `APGIC_COMM_JOIN_POLICY_VERSION`. Register the adapter's Ed25519 **public** key under `APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON` with the exact connector UUID and key ID. The private signing key must **never** be deployed to APGIC.

In LiveKit, configure webhooks to `https://<provider-origin>/v1/livekit-webhooks`. Configure TLS, ICE/TURN/firewall rules appropriate for participant networks. Ensure only the room adapter has the media API secret. A transport/media server can be replaced without changing APGIC's canonical domain.

Build: `docker build -t apgic-communication-reference ./services/communication-reference`. The Docker build pins the direct LiveKit JS SDK version; for a production release, record the built image digest and scan transitive dependencies before publishing. Mount durable `/srv/state`.

## Acceptance gates (NOT bypassable)

- Both **real, separate** authorized participants obtain role-scoped credentials; a third person and stale/revoked entitlement cannot get a grant.
- Two devices/browsers publish and receive audio/video through a **running LiveKit server** on a real network. Prove connection, reconnect after dropped link, reauthorization if the grant expired, voluntary end and technical loss.
- Confirm APGIC receives signed JOINED/STARTED/LEFT/RECOVERY/ENDED events without replaying or manufacturing status. Terminated early, one-sided end, unknown participants and invalid signatures remain non-completed.
- Restart the APGIC API process and read the same provider-confirmed completed result from **PostgreSQL**, with no browser-local status dependency. Also restart the bridge and prove its pending outbox resumes without duplication.
- Validate cancellation/rescheduling via the canonical booking/payment owners; ensure prior booking, ledger/evidence and technical failure history remain immutable.
- Repeat on supported native iOS/Android flows, especially interruption, audio route and foreground/background behavior.

`go test -race ./...` verifies connector/issuer/bridge rules against **simulated signed provider events**. Those tests are useful but are **not a real two-party media certification**. Until the external WSS SFU, actual participants and reboot proof are exercised, APGIC-COMM-001/002, APGIC-CONSULT-001/002 and APGIC-MOBILE-009 must not be marked fully VERIFIED.

## Operational cautions

Do not return a join URL until APGIC has atomically persisted its entitlement-bound authorization. Do not make native or browser reports into authoritative completion events. Do not automatically retry payment. Keep the outbox on a durable volume; loss of that volume means recovery evidence must come from the upstream media provider, never from invented events.

## Single-writer production safety

The JSON reference adapter is intentionally **one process per durable outbox directory**. At boot, it takes an exclusive OS file lock before processing media callbacks or retrying signed events. A rolling deployment must stop the previous instance before starting a replacement against the same volume; if the lock is held, the new process fails closed instead of forking signed sequence numbers. Do not mount this directory over a filesystem that cannot provide reliable file locks. Horizontal scale requires a different CommunicationProvider implementation with distributed transactional outbox/leases; adding replicas or suppressing this lock is not release evidence. `TestDurableProviderOutboxHasOnlyOneProcessOwner` checks duplicate-denial and restart acquisition.

## Deployable media image proof

The official pinned `livekit-client@2.22.3` bundle is `dist/livekit-client.umd.js` (Rollup UMD output and package `main`). Do not substitute a nonexistent `.umd.min.js` path. The mandatory `communication-reference` GitHub job builds the **whole Docker image**, not only source-level Go tests, so the exact bundled browser SDK and container entrypoint remain release gates. This is still not proof of a running SFU or a two-person media call.
