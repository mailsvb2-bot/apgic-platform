export type DiagnosticCheckState = "PASS" | "WARN" | "FAIL" | "UNKNOWN";

export interface MobileDiagnosticReportV1 {
  contract_version: "mobile-diagnostics-v1";
  platform: "IOS" | "ANDROID";
  app_version: string;
  os_version: string;
  network: {
    state: DiagnosticCheckState;
    transport: "WIFI" | "CELLULAR" | "ETHERNET" | "UNKNOWN";
  };
  push: {
    state: DiagnosticCheckState;
    permission: "GRANTED" | "DENIED" | "RESTRICTED" | "UNKNOWN";
  };
  permissions: {
    microphone: DiagnosticCheckState;
  };
  realtime: {
    state: DiagnosticCheckState;
    phase: string;
    reconnect_attempt: number;
    audio_route: string;
    app_state: string;
  };
  compatibility: {
    state: DiagnosticCheckState;
    client_status: "SUPPORTED" | "DEPRECATED_BUT_SUPPORTED" | "UPDATE_REQUIRED" | "UNKNOWN";
  };
  redaction_applied: true;
  generated_at: string;
}

export interface MobileDiagnosticExportV1 {
  contract_version: "mobile-diagnostic-export-v1";
  export_id: string;
  policy_version: string;
  created_at: string;
  expires_at: string;
  report: MobileDiagnosticReportV1;
}
