# Academy video delivery: two retained modes

One transport selector: `APGIC_ACADEMY_VIDEO_DELIVERY_MODE=direct` (default)
or `APGIC_ACADEMY_VIDEO_DELIVERY_MODE=cdn`. Set it once in the process
environment; restart the API for the change to take effect.

- **Direct**: APGIC's protected first-party Yandex byte-range gateway. It
  authorizes every stream against canonical Academy entitlement and OAuth
  generation, and uses complete byte delivery plus explicit learner
  acknowledgment. No CDN account is required.
- **CDN**: uses the existing `connector.CDNPlaybackCapability` typed grant,
  bounded stream, revocation, signed attestation, certification, and
  `AcademyRecordCDNPlaybackEvidence` in PostgreSQL. No external CDN service
  has been configured or deployed. With no production provider adapter, the
  public CDN video route intentionally returns HTTP 503.

**Future single-line activation means switching the selector only after
provider provisioning.** The provider must first implement the versioned
`CDNPlaybackCapability` contract, be registered as an active `CDN_PROVIDER`
instance with appropriate scopes and trusted signing key, and pass independent
live tests including entitlement revocation and exact source-SHA certification.
No token, origin URL, provider name or hosted CDN protocol is guessed here.
Do not represent this as a working CDN deployment before that happens.

Completion of a lesson with several attached videos is shared Academy state:
direct mode requires complete delivery of *every* assigned file; CDN mode now
requires certified completed playback facts for *every* assigned file before
the lesson can be marked complete. Switching transports cannot grant access,
bypass purchase proof or invent video evidence.
