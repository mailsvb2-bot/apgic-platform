import assert from "node:assert/strict";
import test from "node:test";

import {
  createDiagnosticsExport,
  runDiagnostics,
  type DiagnosticSource,
} from "./diagnostics-center.ts";

function source(): DiagnosticSource {
  return {
    platform: "ANDROID",
    appVersion: "1.0.0-ci",
    osVersion: "15",
    network: { state: "WARN", transport: "CELLULAR" },
    push: { state: "PASS", permission: "GRANTED" },
    microphone: "PASS",
    realtime: {
      state: "WARN",
      phase: "RECONNECTING",
      reconnectAttempt: 2,
      audioRoute: "BLUETOOTH",
      appState: "FOREGROUND",
    },
    compatibility: { state: "PASS", clientStatus: "SUPPORTED" },
    untrustedDebugContext: {
      token: "secret-token",
      password: "secret-password",
      payment_secret: "secret-payment",
      raw_consultation: "private session text",
      raw_transcript: "private transcript",
      raw_audio: "bytes",
      raw_video: "bytes",
      avatar_raw: "bytes",
    },
  };
}

test("diagnostics allowlist drops tokens payment secrets and raw session content", () => {
  const report = runDiagnostics(source(), new Date("2026-09-24T12:00:00Z"));
  const json = JSON.stringify(report);

  for (const forbidden of [
    "secret-token",
    "secret-password",
    "secret-payment",
    "private session text",
    "private transcript",
    "raw_audio",
    "raw_video",
    "avatar_raw",
  ]) {
    assert.equal(json.includes(forbidden), false, forbidden);
  }
  assert.equal(report.redaction_applied, true);
  assert.equal(report.realtime.phase, "RECONNECTING");
});

test("diagnostic export requires explicit confirmation and expires", () => {
  const now = new Date("2026-09-24T12:00:00Z");
  const report = runDiagnostics(source(), now);

  assert.throws(() =>
    createDiagnosticsExport(
      report,
      { exportId: "diag-1", confirmed: false, ttlSeconds: 300 },
      { policyVersion: "mobile-realtime-r3-ci-v1", maxExportTtlSeconds: 900 },
      now,
    ),
  );

  const bundle = createDiagnosticsExport(
    report,
    { exportId: "diag-1", confirmed: true, ttlSeconds: 300 },
    { policyVersion: "mobile-realtime-r3-ci-v1", maxExportTtlSeconds: 900 },
    now,
  );
  assert.equal(bundle.expires_at, "2026-09-24T12:05:00.000Z");
  assert.equal(bundle.report.redaction_applied, true);
});

test("diagnostic export cannot exceed policy ttl", () => {
  const now = new Date("2026-09-24T12:00:00Z");
  const report = runDiagnostics(source(), now);
  assert.throws(() =>
    createDiagnosticsExport(
      report,
      { exportId: "diag-2", confirmed: true, ttlSeconds: 901 },
      { policyVersion: "mobile-realtime-r3-ci-v1", maxExportTtlSeconds: 900 },
      now,
    ),
  );
});
