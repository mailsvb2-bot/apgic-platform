import type {
  DiagnosticCheckState,
  MobileDiagnosticExportV1,
  MobileDiagnosticReportV1,
} from "../../../packages/contracts/src/mobile-diagnostics";

export interface DiagnosticSource {
  platform: "IOS" | "ANDROID";
  appVersion: string;
  osVersion: string;
  network: {
    state: DiagnosticCheckState;
    transport: "WIFI" | "CELLULAR" | "ETHERNET" | "UNKNOWN";
  };
  push: {
    state: DiagnosticCheckState;
    permission: "GRANTED" | "DENIED" | "RESTRICTED" | "UNKNOWN";
  };
  microphone: DiagnosticCheckState;
  realtime: {
    state: DiagnosticCheckState;
    phase: string;
    reconnectAttempt: number;
    audioRoute: string;
    appState: string;
  };
  compatibility: {
    state: DiagnosticCheckState;
    clientStatus: "SUPPORTED" | "DEPRECATED_BUT_SUPPORTED" | "UPDATE_REQUIRED" | "UNKNOWN";
  };
  untrustedDebugContext?: Record<string, unknown>;
}

export interface DiagnosticsExportPolicy {
  policyVersion: string;
  maxExportTtlSeconds: number;
}

export interface DiagnosticsExportRequest {
  exportId: string;
  confirmed: boolean;
  ttlSeconds: number;
}

export function runDiagnostics(source: DiagnosticSource, now: Date): MobileDiagnosticReportV1 {
  if (!source.appVersion || !source.osVersion || Number.isNaN(now.getTime())) {
    throw new Error("invalid diagnostic source");
  }
  if (!Number.isInteger(source.realtime.reconnectAttempt) || source.realtime.reconnectAttempt < 0) {
    throw new Error("invalid reconnect attempt");
  }

  return {
    contract_version: "mobile-diagnostics-v1",
    platform: source.platform,
    app_version: source.appVersion,
    os_version: source.osVersion,
    network: { ...source.network },
    push: { ...source.push },
    permissions: { microphone: source.microphone },
    realtime: {
      state: source.realtime.state,
      phase: source.realtime.phase,
      reconnect_attempt: source.realtime.reconnectAttempt,
      audio_route: source.realtime.audioRoute,
      app_state: source.realtime.appState,
    },
    compatibility: {
      state: source.compatibility.state,
      client_status: source.compatibility.clientStatus,
    },
    redaction_applied: true,
    generated_at: now.toISOString(),
  };
}

export function createDiagnosticsExport(
  report: MobileDiagnosticReportV1,
  request: DiagnosticsExportRequest,
  policy: DiagnosticsExportPolicy,
  now: Date,
): MobileDiagnosticExportV1 {
  if (
    !request.confirmed ||
    !request.exportId ||
    !policy.policyVersion ||
    !Number.isInteger(request.ttlSeconds) ||
    request.ttlSeconds <= 0 ||
    request.ttlSeconds > policy.maxExportTtlSeconds ||
    policy.maxExportTtlSeconds <= 0 ||
    Number.isNaN(now.getTime())
  ) {
    throw new Error("diagnostic export denied");
  }

  return {
    contract_version: "mobile-diagnostic-export-v1",
    export_id: request.exportId,
    policy_version: policy.policyVersion,
    created_at: now.toISOString(),
    expires_at: new Date(now.getTime() + request.ttlSeconds * 1000).toISOString(),
    report,
  };
}
