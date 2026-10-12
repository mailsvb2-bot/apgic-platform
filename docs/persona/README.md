# APGIC Persona Companion — feature launch boundary

Sources: v7 FINAL §§48–51, 109, 218; APGIC-PRIV-001 and APGIC-DATA-001.

APGIC owns user Identity, published/rights-cleared persona catalog,
AvatarSession business lifecycle and explicit human handoff facts.
External PersonaProvider owns inference/avatar/voice/media runtime.
No VPR code or donor DB may be copied as APGIC canonical business truth.

**Default OFF.** Migration 000032 must run before enabling the feature.
Provision all settings together:
`APGIC_PERSONA_ENABLED=1`,
`APGIC_PERSONA_PROVIDER_ORIGIN=https://<certified-provider>`,
`APGIC_PERSONA_PROVIDER_TOKEN=<scoped-secret>`,
`APGIC_PERSONA_TRANSPORT_HOSTS=<explicit-trusted-hostnames>`,
`APGIC_PERSONA_PROVIDER_INSTANCE_ID=<active-persona-connector-UUID>`,
`APGIC_PERSONA_POLICY_VERSION=<approved-version>`.

The provider connector calls POST /v1/persona-sessions (scoped context
only), POST /v1/persona-sessions/close (provider reference only). No
conversation transcript, raw audio, diagnostics, inferred clinical
data, private client notes, booking or payment authority are passed.

Client APIs: GET /v1/personas, POST /v1/avatar-sessions,
GET /v1/avatar-sessions/{id}, POST /v1/avatar-sessions/{id}/actions.
Cookie-authenticated ownership, idempotency and private ephemeral mode
are enforced by API + SQL. Current handoff creates no implicit booking
or data transfer; the user must create a separate HelpIntent through the
canonical qualification/matching pipeline.

No fake personas or live providers are seeded. If no approved provider
exists, show unavailable and offer a separate human specialist route.

**NOT PRODUCTION CERTIFIED:** Requires provider/operator selection,
rights-cleared catalogue, signed provider event verification, real
native/web transport proof, provider deletion/TTL and kill-switch
tests, safety/anti-dependency evaluation, incident runbook and
consent/economic model. Do not silently mark VERIFIED or RELEASED.

The chosen language, voice/visual profiles and **style** are immutable
business context and forwarded to PersonaProvider; style cannot silently
be ignored by the connector. Actual policy/model adherence requires provider
evaluation before release.

## Model registry, launch admission and kill switch

Before publishing any persona or setting APGIC_PERSONA_ENABLED=1,
provision one reviewed `persona_model_deployments` row for its provider and
policy version, containing model/config/safety/eval evidence, positive
per-identity hourly start budget and concurrent-session limit. The schema
has **no fixture or silent default model**. An append-only
`persona_model_disable_decisions` record immediately stops new starts
for that policy without rewriting history. Re-enable only through a newly
approved policy version. PostgreSQL admission is serialized by Identity
under a transaction-level advisory lock to prevent concurrent budget
bypass. Releasing an active session requires the canonical END flow.

## Actual conversation: private model-backed PersonaProvider

The separate process `backend/cmd/persona-provider` is a minimal, real
TEXT/VOICE **conversation** runtime backed by an explicitly selected
OpenAI-compatible Chat Completions model. It is not a stub or a VPR release.
It does not claim to produce animated video, WebRTC, high-quality TTS,
clinical therapy or production certification.

The provider **will not start** without all of:

```sh
PERSONA_PROVIDER_SERVICE_TOKEN=<random-service-secret-32+-chars>
PERSONA_PROVIDER_LLM_URL=https://<approved-inference-host>/v1/chat/completions
PERSONA_PROVIDER_LLM_API_KEY=<private-model-key>
PERSONA_PROVIDER_LLM_MODEL=<reviewed-model-identifier>
PERSONA_PROVIDER_POLICY_VERSION=<same-approved-policy-version>
```

Run it as an independent supervised process (`cd backend && go run ./cmd/persona-provider`),
bound to `127.0.0.1:8788` only. Put a TLS reverse proxy in front with
an allowlist that exposes **only the APGIC server as a caller**.
In APGIC use `APGIC_PERSONA_PROVIDER_ORIGIN=https://<that-proxy>`,
`APGIC_PERSONA_PROVIDER_TOKEN` = service token,
`APGIC_PERSONA_TRANSPORT_HOSTS` = approved external video hosts
(required by the legacy connector), and the matching active connector UUID and
policy version. Never put inference keys in browser/mobile bundles.

APGIC owns identity and admission. On each POST to
`/v1/avatar-sessions/{id}/turns` it validates the client session cookie,
owner, current ACTIVE state, model deployment, provider, rights and
kill-switch, then exchanges a **single** ephemeral message with the provider.
The provider calls the configured model using private credentials, verifies
an actual completion (`source=MODEL_GENERATED`) and returns its text.
A missing/invalid provider answer is a 503, **not** a scripted AI reply.

The provider holds at most 100 simultaneous sessions, each for 12 minutes
and up to 16 turns; 2,000 UTF-8 input bytes per turn, 320 generated tokens
per answer, 12 most recent turn messages in memory. New sessions still pass
APGIC's per-identity PostgreSQL concurrency/hourly budgets.
Close and TTL expiry cancel in-flight calls, invalidate references and
erase provider history. No raw message, audio/video or prompt is written
to APGIC PostgreSQL by this flow. Client-side text exists only in live UI
state and is cleared on END/HANDOFF/SAFETY. Do not enable persistent
HTTP-body/access capture on intermediary proxies.

The browser can do voice input/output with the Web Speech API when supported:
microphone speech becomes a text turn to the real LLM, its answer is voiced
by the browser. Browser speech recognition may process audio with the
browser vendor: disclose this to the user before voice input, never claim
the browser provider is zero-retention. Mobile currently supports full
model-backed text, plus external links for voice/video where an independently
certified transport is available. Native microphone/TTS is **not** claimed.

### External image/voice rights

Migration 000033 introduces an append-only reviewed license record per
persona, storing its rights holder reference, SHA-256 evidence digest,
reviewer, permitted TEXT/VOICE/appearance uses and validity deadline.
A non-empty original `rights_evidence_ref` alone is **not** approval.
A rights revocation immediately blocks starts and new turns.
Publishing avatars impersonating actual people requires affirmative
reviewed rights; no real person's appearance or voice is bundled or seeded.
Rights evidence and authorized persona publication are operator duties.

### Safety, deletion and release gate

LLM safety prompt discloses AI status, forbids diagnostics/therapy claims,
fabricated qualifications, dependency and paid coercion, and points to
real human/emergency help when appropriate. Manual safety escalation and
human handoff revoke conversation access, close provider resources and
never copy transcripts into HelpIntent. Provider failure, safety review,
response quality, adversarial jailbreak tests, voice privacy and
operator-mediated closure after transport outages **still require
real-provider evidence**.

This process is a working *conditional* path: genuine model replies occur
when a compatible authorized endpoint, reviewed model and rights-cleared
persona are actually provisioned. No production LLM credentials or approved
rights are included here. Do not label APGIC/VPR production-ready solely
because contract/mock tests pass. A delivery-uncertain provider close is
reported explicitly rather than falsely declaring remote deletion complete.

## Operational replay and real provider readiness

The model-backed sidecar keeps an ephemeral APGIC-session-to-provider-session binding: repeated OPEN with identical reviewed context returns the same lease, while mismatched persona/style/mode is rejected. After user CLOSE or TTL expiry, an opaque temporary tombstone blocks replay of the same APGIC session. A failed inference attempt consumes the per-session turn budget; no unlimited provider retries. Provider health is probed before APGIC advertises an avatar as selectable. This health check confirms only that the model-backed gateway is reachable; it is **not** a claim of external LLM inference success or model safety certification. Every answer still requires an actual, independently validated model completion.

## Mandatory per-persona runtime identity and capability intersection

A single global style prompt is **not** a sufficient implementation of a
selected AI character. Provider startup also requires
`PERSONA_PROVIDER_PERSONAS_JSON`: an explicit, operator-reviewed manifest
with an entry for **every** published and licensed Persona ID to be served.
A fictional example (not seeded, not a rights approval):

```json
{
  "00000000-0000-0000-0000-000000000201": {
    "label": "Добрый Ёж",
    "role": "Добрый вымышленный ёж, который внимательно слушает, задаёт короткие вопросы и не притворяется врачом.",
    "language": "ru",
    "voice_profile": "reviewed-neutral-browser-voice",
    "visual_profile": "reviewed-fictional-illustration",
    "allows_text": true,
    "allows_voice": true
  }
}
```

This profile must match the independently approved APGIC `persona_catalog`
identity, voice/visual profiles, language, and reviewed licensing
(`persona_rights_reviews`). The runtime rejects unknown, mismatched or
unsupported profiles at OPEN. The selected character's reviewed identity
and tone are injected into the actual model request under the non-negotiable
model safety policy, rather than being ignored. The provider's protected
`GET /v1/persona-catalog` exposes only capabilities, never the private
role prompt. APGIC offers only the intersection of operator-reviewed
provider capabilities and its rights-cleared published catalog; if no
intersection exists, the avatar entry is disabled and HelpIntent still works.

Native voice and real-time animated appearance require an independently
certified transport; the current model-backed sidecar advertises **TEXT and
VOICE as model dialogue modes**, not a working synthetic face. Browser VOICE
uses the browser's speech recognition and speech synthesis, which may rely
on external browser-vendor processing. Do not promise voice-profile fidelity,
zero-retention inference, VPR parity or production readiness without live
end-to-end proof and provider terms validation.

## Reload, restart and provider-liveness recovery

The protected \`GET /v1/avatar-sessions/current\` endpoint returns at most one
currently ACTIVE, unexpired session owned by the browser/mobile cookie. It
never returns a provider session reference, inference credentials, history,
prompt or audio. If no authorized owner cookie exists, the client treats 401
as first use. Text/voice sessions can resume after a UI reload while their
provider lease is still alive. The previous page's temporary messages are
deliberately lost; the short provider-side conversation memory stays only
until the original lease expires or is closed.

Restoration requires both PostgreSQL authority (published and rights-cleared
persona, approved model, kill-switch, state, time budget) **and** positive
evidence from provider \`POST /v1/persona-sessions/status\`. A definite absence
after provider restart records ENDED and permits a new session. A provider
network error returns 503, rather than guessing that the old lease was
deleted. A revoked policy blocks new turns, closes the business session and
attempts provider cleanup. Remote failure is explicitly marked uncertain.
There is no implicit video reauthorization: VIDEO needs a fresh,
provider-approved short-lived launch capability.

A browser refresh and a lost provider response must never produce a
fabricated model answer or duplicate billing. Provider is deliberately
in-memory and should not be load-balanced without a session-affine routing
contract; its independent 12-minute TTL and bounded revocation fence are
defense-in-depth, not proof of persistent remote erasure. A future durable
provider-cleanup reconciliation path is necessary before launch certification.
