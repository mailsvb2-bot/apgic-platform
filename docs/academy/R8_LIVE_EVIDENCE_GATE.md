# R8 live provider evidence gate (not release authorization)

The ordinary green CI run is **not** evidence of actual Yandex OAuth, video bytes, a signed learner session, a captured PSP transaction, or Finance reconciliation.

## Explicit real-provider smoke (never use a fake provider or log credentials)

Use a dedicated nonproduction Yandex account with an author-owned MP4 >=128 KiB and valid metadata/fingerprint, on an authorized environment with Go and network access. Supply `APGIC_TEST_YANDEX_OAUTH_TOKEN` and `APGIC_TEST_YANDEX_VIDEO_PATH` using the environment's secret manager, **never as CLI arguments, committed files, issue text or CI logs**.

From `backend/`, execute:

```sh
APGIC_REQUIRE_R8_LIVE_PROOF=true go test ./internal/academy ./internal/httpapi -run '^TestLive(YandexVerifiedVideoRange|AcademyYandexGatewaySeekAndRevocation|AcademyYandexGatewayStopsInFlightAfterRevocation)$' -count=1 -v
```

With `APGIC_REQUIRE_R8_LIVE_PROOF=true`, missing token or video **fails**, rather than silently skipping. Without it, the existing opt-in tests still skip when no credentials are provided; standard CI remains a code-regression gate only.

These tests exercise real cloud range bytes through the hardened media transport, post-revocation denial and cancellation of continued byte copying. **They do not establish** a genuine independently signed learner/author identity, real Entitlement revocation persisted in PostgreSQL, actual client browser playback, real captured PSP purchase or Finance reconciliation. Those are separate release blockers.

## Remaining operator evidence before removing Draft from #343

1. Record exact commit SHA, deployed SHA, test invocation, environment identifier, test results, and redacted provider response status; keep tokens, temporary download URLs and user identities out of evidence.
2. Perform real author Yandex OAuth against the actual deployed API with a signed author session; import, map and verify an MP4.
3. With a *different signed learner* and independently verified externally captured PSP transaction, prove canonical Commerce -> Entitlement -> Academy enrollment and playback; confirm Finance is a distinct reconciliation owner (not simulated revenue).
4. Revoke the entitlement in canonical PostgreSQL/Commerce lifecycle and confirm that new range requests fail and an already-running stream is interrupted. Re-run for OAuth disconnect and account reconnect.
5. Confirm no provider URL/token in browser JSON, redirect Location, referrer, logs or telemetry. Record browser and native device results.

Do **not** set `APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED=true` or merge #343 solely on a green CI run or on the real-provider smoke above. VIDEO publication stays default-off until the complete protected journey has been independently evidenced.
