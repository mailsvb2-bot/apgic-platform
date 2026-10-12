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
