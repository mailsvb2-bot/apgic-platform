# R8 live provider evidence gate (not release authorization)

The ordinary green CI run is **not** evidence of actual Yandex OAuth, video bytes, a signed learner session, a captured PSP transaction, or Finance reconciliation.

## Two production delivery modes (main, 2026-11-17)

`APGIC_ACADEMY_VIDEO_DELIVERY_MODE=direct` (default) registers the
first-party Yandex-to-APGIC byte-range learner route. This route is implemented
in production code, but a green CI run does not authorize live VIDEO course
publication: an exact-SHA, independently accepted Yandex/payment/revocation
journey, active entitlements and publication opt-in are still mandatory.

`APGIC_ACADEMY_VIDEO_DELIVERY_MODE=cdn` is a separately preserved transport.
Existing `CDNPlaybackCapability`, typed grant/stream/revocation/external
attestation methods, certification and PostgreSQL evidence are retained.
Until a real CDN provider adapter is connected and independently certified,
the learner HTTP route intentionally responds 503; merely changing the
environment selector cannot bypass this boundary.

Both modes use Academy as the only course, entitlement, media mapping and
progress authority. Complete delivery in direct mode and certified playback
evidence in CDN mode must cover every video assigned to the lesson.
See `R8_VIDEO_DELIVERY_MODES.md` for the switch and activation prerequisites.

## Explicit real-provider smoke (never use a fake provider or log credentials)

Use a dedicated nonproduction Yandex account with an author-owned MP4 >=128 KiB and valid metadata/fingerprint, on an authorized environment with Go and network access. Supply `APGIC_TEST_YANDEX_OAUTH_TOKEN` and `APGIC_TEST_YANDEX_VIDEO_PATH` using the environment's secret manager, **never as CLI arguments, committed files, issue text or CI logs**.

From `backend/`, execute:

```sh
APGIC_REQUIRE_R8_LIVE_PROOF=true go test ./internal/academy ./internal/httpapi -run '^TestLive(YandexVerifiedVideoRange|AcademyYandexGatewaySeekAndRevocation|AcademyYandexGatewayStopsInFlightAfterRevocation)$' -count=1 -v
```

With `APGIC_REQUIRE_R8_LIVE_PROOF=true`, missing token or video **fails**, rather than silently skipping. Without it, the existing opt-in tests still skip when no credentials are provided; standard CI remains a code-regression gate only.

These tests exercise real cloud range bytes through the hardened media transport, post-revocation denial and cancellation of continued byte copying. **They do not establish** a genuine independently signed learner/author identity, real Entitlement revocation persisted in PostgreSQL, actual client browser playback, real captured PSP purchase or Finance reconciliation. Those are separate release blockers.

## Remaining independent operator evidence before live VIDEO publication

1. Record exact commit SHA, deployed SHA, test invocation, environment identifier, test results, and redacted provider response status; keep tokens, temporary download URLs and user identities out of evidence.
2. Perform real author Yandex OAuth against the actual deployed API with a signed author session; import, map and verify an MP4.
3. With a *different signed learner* and independently verified externally captured PSP transaction, prove canonical Commerce -> Entitlement -> Academy enrollment and playback; confirm Finance is a distinct reconciliation owner (not simulated revenue).
4. Revoke the entitlement in canonical PostgreSQL/Commerce lifecycle and confirm that new range requests fail and an already-running stream is interrupted. Re-run for OAuth disconnect and account reconnect.
5. Confirm no provider URL/token in browser JSON, redirect Location, referrer, logs or telemetry. Record browser and native device results.

Do **not** set `APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED=true` or treat an already merged feature PR as independent release authorization solely on a green CI run or on the real-provider smoke above. VIDEO publication stays default-off until the complete protected journey has been independently evidenced.

## CI startup failure is not a test verdict

A GitHub Actions `startup_failure` with **zero jobs** (for example,
[run 42067470439](https://github.com/mailsvb2-bot/apgic-platform/actions/runs/42067470439))
does not establish application or test failure **and is not a PASS**. Do not
count a prior green workflow on an older SHA as an acceptable final-HEAD gate.

Record the failed run ID, exact SHA and the absence of job results, and
require a **new successful full CI run against the current main HEAD**.
Do not skip Android/iOS/PostgreSQL jobs or weaken the release checks to
compensate for GitHub orchestration failure. Manual Yandex provider acceptance
remains an independent, real-secret opt-in test and does not replace CI.

## CDN playback contract v2: byte delivery is mandatory

The breaking v2 `CDNPlaybackCapability` requires `OpenPlaybackStream` in
addition to grant issuance, signed attestation verification and active-session
revocation. A provider that can issue a grant but cannot return bytes is not a
valid playback provider. The contract version was advanced to v2 rather than silently changing the v1 interface.

The provider returns only an opaque `io.ReadCloser` plus status, video MIME
type, content length and (for 206) the exact Content-Range. The type has no
field for an origin URL, OAuth token, cookie or arbitrary upstream headers.
A full object is exactly 200; a single inclusive range is exactly 206. The
gateway must generate 416 before provider I/O for an unsatisfiable range.
Provider implementations must close active bodies synchronously when the
matching grant is revoked. This is a contract and validation layer only: no
production adapter is registered yet, so the learner route remains HTTP 503.
