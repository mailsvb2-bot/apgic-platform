declare const __DEV__: boolean;

export type NativeAuthzSurface = "IOS" | "ANDROID";

export type NativeTenantIsolationE2EConfig = {
  baseURL: string;
  sessionCookie: string;
  ownOrganizationID: string;
  foreignOrganizationID: string;
  foreignPrivateMarker: string;
  surface: NativeAuthzSurface;
};

export type NativeTenantIsolationE2EResult = {
  sameTenantAllowed: true;
  crossTenantDenied: true;
  forgedContextDenied: true;
  privateDisclosureBlocked: true;
  crossTenantAuditPersisted: true;
  forgedContextAuditPersisted: true;
};

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

export type NativeAuthzFetch = (
  url: string,
  options?: FetchOptions,
) => Promise<FetchResponse>;

const defaultFetch: NativeAuthzFetch = (url, options) => fetch(url, options);

function canonicalBaseURL(raw: string): string {
  const candidate = raw.trim().replace(/\/$/, "");
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error("MOBILE_AUTHZ_BASE_URL_INVALID");
  }
  const loopback =
    parsed.protocol === "http:" &&
    (parsed.hostname === "127.0.0.1" || parsed.hostname === "localhost");
  if (
    (parsed.protocol !== "https:" && !loopback) ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== "" ||
    (parsed.pathname !== "" && parsed.pathname !== "/")
  ) {
    throw new Error("MOBILE_AUTHZ_BASE_URL_INVALID");
  }
  return candidate;
}

function requireString(value: string, code: string): string {
  const normalized = value.trim();
  if (!normalized) {
    throw new Error(code);
  }
  return normalized;
}

function asObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("MOBILE_AUTHZ_RESPONSE_INVALID");
  }
  return value as Record<string, unknown>;
}

async function requestProfile(
  config: NativeTenantIsolationE2EConfig,
  request: NativeAuthzFetch,
  organizationID: string,
  contextID: string,
  correlationID: string,
): Promise<{status: number; payload: Record<string, unknown>}> {
  const baseURL = canonicalBaseURL(config.baseURL);
  const response = await request(
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/private-profile`,
    {
      method: "GET",
      credentials: "include",
      headers: {
        Accept: "application/json",
        Cookie: config.sessionCookie,
        "X-Organization-Context": contextID,
        "X-Correlation-Id": correlationID,
      },
    },
  );
  return {status: response.status, payload: asObject(await response.json())};
}

async function requireAudit(
  config: NativeTenantIsolationE2EConfig,
  request: NativeAuthzFetch,
  correlationID: string,
  expectedReason: string,
): Promise<void> {
  const baseURL = canonicalBaseURL(config.baseURL);
  const response = await request(
    `${baseURL}/e2e/authz-audit/${encodeURIComponent(correlationID)}`,
    {method: "GET", headers: {Accept: "application/json"}},
  );
  if (response.status !== 200) {
    throw new Error("MOBILE_AUTHZ_AUDIT_NOT_PERSISTED");
  }
  const payload = asObject(await response.json());
  if (
    payload.correlation_id !== correlationID ||
    payload.reason !== expectedReason ||
    payload.decision !== "DENY"
  ) {
    throw new Error("MOBILE_AUTHZ_AUDIT_INVALID");
  }
}

export async function runNativeTenantIsolationE2E(
  config: NativeTenantIsolationE2EConfig,
  request: NativeAuthzFetch = defaultFetch,
): Promise<NativeTenantIsolationE2EResult> {
  if (typeof __DEV__ !== "undefined" && !__DEV__) {
    throw new Error("MOBILE_AUTHZ_E2E_DEBUG_ONLY");
  }

  const ownOrganizationID = requireString(
    config.ownOrganizationID,
    "MOBILE_AUTHZ_OWN_ORGANIZATION_REQUIRED",
  );
  const foreignOrganizationID = requireString(
    config.foreignOrganizationID,
    "MOBILE_AUTHZ_FOREIGN_ORGANIZATION_REQUIRED",
  );
  const foreignPrivateMarker = requireString(
    config.foreignPrivateMarker,
    "MOBILE_AUTHZ_FOREIGN_MARKER_REQUIRED",
  );
  if (ownOrganizationID === foreignOrganizationID) {
    throw new Error("MOBILE_AUTHZ_ORGANIZATIONS_MUST_DIFFER");
  }
  if (config.surface !== "IOS" && config.surface !== "ANDROID") {
    throw new Error("MOBILE_AUTHZ_SURFACE_INVALID");
  }
  requireString(config.sessionCookie, "MOBILE_AUTHZ_SESSION_REQUIRED");

  const prefix = `auth001-native-${config.surface.toLowerCase()}`;
  const own = await requestProfile(
    config,
    request,
    ownOrganizationID,
    ownOrganizationID,
    `${prefix}-own`,
  );
  if (
    own.status !== 200 ||
    own.payload.id !== ownOrganizationID ||
    typeof own.payload.name !== "string" ||
    !own.payload.name
  ) {
    throw new Error("MOBILE_AUTHZ_SAME_TENANT_ALLOW_FAILED");
  }

  const crossCorrelation = `${prefix}-cross-tenant`;
  const cross = await requestProfile(
    config,
    request,
    foreignOrganizationID,
    ownOrganizationID,
    crossCorrelation,
  );
  const crossSerialized = JSON.stringify(cross.payload);
  if (
    cross.status !== 403 ||
    cross.payload.code !== "AUTH_CROSS_TENANT_DENY" ||
    crossSerialized.includes(foreignPrivateMarker)
  ) {
    throw new Error("MOBILE_AUTHZ_CROSS_TENANT_DENY_FAILED");
  }
  await requireAudit(
    config,
    request,
    crossCorrelation,
    "AUTH_CROSS_TENANT_DENY",
  );

  const forgedCorrelation = `${prefix}-forged-context`;
  const forged = await requestProfile(
    config,
    request,
    foreignOrganizationID,
    foreignOrganizationID,
    forgedCorrelation,
  );
  const forgedSerialized = JSON.stringify(forged.payload);
  if (
    forged.status !== 403 ||
    forged.payload.code !== "AUTH_TENANT_CONTEXT_DENIED" ||
    forgedSerialized.includes(foreignPrivateMarker)
  ) {
    throw new Error("MOBILE_AUTHZ_FORGED_CONTEXT_DENY_FAILED");
  }
  await requireAudit(
    config,
    request,
    forgedCorrelation,
    "AUTH_TENANT_CONTEXT_DENIED",
  );

  return {
    sameTenantAllowed: true,
    crossTenantDenied: true,
    forgedContextDenied: true,
    privateDisclosureBlocked: true,
    crossTenantAuditPersisted: true,
    forgedContextAuditPersisted: true,
  };
}
