declare const __DEV__: boolean;

export type MobileInstallationPlatform = "IOS" | "ANDROID";

export type MobileInstallation = {
  id: string;
  identity_id: string;
  platform: MobileInstallationPlatform;
  push_endpoint?: string;
  push_generation: number;
  state: "ACTIVE" | "REVOKED";
};

export type InstallationClientConfig = {
  baseURL: string;
  sessionCookie?: string;
};

export type InstallationE2EConfig = InstallationClientConfig & {
  sessionCookie: string;
  installationID: string;
  platform: MobileInstallationPlatform;
};

export type InstallationE2EResult = {
  identityID: string;
  pushGeneration: number;
  state: "REVOKED";
};

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

export type InstallationFetch = (
  url: string,
  options?: FetchOptions,
) => Promise<FetchResponse>;

const defaultFetch: InstallationFetch = (url, options) => fetch(url, options);

function canonicalBaseURL(raw: string): string {
  const candidate = raw.trim().replace(/\/$/, "");
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error("MOBILE_INSTALLATION_BASE_URL_INVALID");
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
    parsed.pathname !== ""
  ) {
    throw new Error("MOBILE_INSTALLATION_BASE_URL_INVALID");
  }
  return candidate;
}

export function requireInstallation(value: unknown): MobileInstallation {
  if (!value || typeof value !== "object") {
    throw new Error("MOBILE_INSTALLATION_RESPONSE_INVALID");
  }
  const candidate = value as Partial<MobileInstallation>;
  if (
    typeof candidate.id !== "string" ||
    typeof candidate.identity_id !== "string" ||
    (candidate.platform !== "IOS" && candidate.platform !== "ANDROID") ||
    typeof candidate.push_generation !== "number" ||
    !Number.isInteger(candidate.push_generation) ||
    candidate.push_generation < 1 ||
    (candidate.state !== "ACTIVE" && candidate.state !== "REVOKED") ||
    (candidate.push_endpoint !== undefined &&
      typeof candidate.push_endpoint !== "string")
  ) {
    throw new Error("MOBILE_INSTALLATION_RESPONSE_INVALID");
  }
  return candidate as MobileInstallation;
}

async function requestJSON(
  config: InstallationClientConfig,
  request: InstallationFetch,
  path: string,
  method: string,
  expectedStatuses: readonly number[],
  body?: Record<string, string>,
): Promise<{status: number; payload: unknown}> {
  const baseURL = canonicalBaseURL(config.baseURL);
  const headers: Record<string, string> = {Accept: "application/json"};
  if (config.sessionCookie) {
    headers.Cookie = config.sessionCookie;
  }
  if (body) {
    headers["Content-Type"] = "application/json";
  }
  const response = await request(baseURL + path, {
    method,
    headers,
    credentials: "include",
    ...(body ? {body: JSON.stringify(body)} : {}),
  });
  if (!expectedStatuses.includes(response.status)) {
    throw new Error(`MOBILE_INSTALLATION_HTTP_${response.status}`);
  }
  return {status: response.status, payload: await response.json()};
}

export async function registerMobileInstallation(
  config: InstallationClientConfig,
  input: {
    id: string;
    platform: MobileInstallationPlatform;
    pushEndpoint: string;
  },
  request: InstallationFetch = defaultFetch,
): Promise<{installation: MobileInstallation; idempotent: boolean}> {
  if (
    !input.id.trim() ||
    (input.platform !== "IOS" && input.platform !== "ANDROID") ||
    !input.pushEndpoint.trim()
  ) {
    throw new Error("MOBILE_INSTALLATION_REGISTER_INVALID");
  }
  const response = await requestJSON(
    config,
    request,
    "/v1/mobile/installations",
    "POST",
    [200, 201],
    {
      id: input.id,
      platform: input.platform,
      push_endpoint: input.pushEndpoint,
    },
  );
  return {
    installation: requireInstallation(response.payload),
    idempotent: response.status === 200,
  };
}

export async function rotateMobilePushEndpoint(
  config: InstallationClientConfig,
  installationID: string,
  pushEndpoint: string,
  request: InstallationFetch = defaultFetch,
): Promise<MobileInstallation> {
  if (!installationID.trim() || !pushEndpoint.trim()) {
    throw new Error("MOBILE_INSTALLATION_ROTATE_INVALID");
  }
  const response = await requestJSON(
    config,
    request,
    `/v1/mobile/installations/${encodeURIComponent(installationID)}/push-endpoint`,
    "PATCH",
    [200],
    {push_endpoint: pushEndpoint},
  );
  return requireInstallation(response.payload);
}

export async function revokeMobileInstallation(
  config: InstallationClientConfig,
  installationID: string,
  request: InstallationFetch = defaultFetch,
): Promise<MobileInstallation> {
  if (!installationID.trim()) {
    throw new Error("MOBILE_INSTALLATION_REVOKE_INVALID");
  }
  const response = await requestJSON(
    config,
    request,
    `/v1/mobile/installations/${encodeURIComponent(installationID)}/revoke`,
    "POST",
    [200],
  );
  return requireInstallation(response.payload);
}

export async function listMobileInstallations(
  config: InstallationClientConfig,
  request: InstallationFetch = defaultFetch,
): Promise<MobileInstallation[]> {
  const response = await requestJSON(
    config,
    request,
    "/v1/mobile/installations",
    "GET",
    [200],
  );
  if (!response.payload || typeof response.payload !== "object") {
    throw new Error("MOBILE_INSTALLATION_LIST_INVALID");
  }
  const values = (response.payload as {installations?: unknown}).installations;
  if (!Array.isArray(values)) {
    throw new Error("MOBILE_INSTALLATION_LIST_INVALID");
  }
  return values.map(requireInstallation);
}

export async function runInstallationE2ELifecycle(
  config: InstallationE2EConfig,
  request: InstallationFetch = defaultFetch,
): Promise<InstallationE2EResult> {
  if (typeof __DEV__ !== "undefined" && !__DEV__) {
    throw new Error("MOBILE_INSTALLATION_E2E_DISABLED");
  }
  if (
    !config.sessionCookie ||
    !config.installationID ||
    (config.platform !== "IOS" && config.platform !== "ANDROID")
  ) {
    throw new Error("MOBILE_INSTALLATION_E2E_CONFIG_INVALID");
  }

  const clientConfig: InstallationClientConfig = {
    baseURL: config.baseURL,
    sessionCookie: config.sessionCookie,
  };
  const tokenA = `e2e-${config.platform.toLowerCase()}-${config.installationID}-a`;
  const tokenB = `e2e-${config.platform.toLowerCase()}-${config.installationID}-b`;

  const registeredResult = await registerMobileInstallation(
    clientConfig,
    {
      id: config.installationID,
      platform: config.platform,
      pushEndpoint: tokenA,
    },
    request,
  );
  const registered = registeredResult.installation;
  if (
    registeredResult.idempotent ||
    registered.id !== config.installationID ||
    registered.platform !== config.platform ||
    registered.state !== "ACTIVE" ||
    registered.push_generation !== 1 ||
    registered.push_endpoint !== tokenA
  ) {
    throw new Error("MOBILE_INSTALLATION_REGISTER_INVARIANT");
  }

  const rotated = await rotateMobilePushEndpoint(
    clientConfig,
    config.installationID,
    tokenB,
    request,
  );
  if (
    rotated.identity_id !== registered.identity_id ||
    rotated.state !== "ACTIVE" ||
    rotated.push_generation !== 2 ||
    rotated.push_endpoint !== tokenB
  ) {
    throw new Error("MOBILE_INSTALLATION_ROTATE_INVARIANT");
  }

  const revoked = await revokeMobileInstallation(
    clientConfig,
    config.installationID,
    request,
  );
  if (
    revoked.identity_id !== registered.identity_id ||
    revoked.state !== "REVOKED" ||
    revoked.push_generation !== 2 ||
    revoked.push_endpoint
  ) {
    throw new Error("MOBILE_INSTALLATION_REVOKE_INVARIANT");
  }

  const listed = await listMobileInstallations(clientConfig, request);
  const persisted = listed.find(
    (candidate) => candidate.id === config.installationID,
  );
  if (
    !persisted ||
    persisted.identity_id !== registered.identity_id ||
    persisted.state !== "REVOKED" ||
    persisted.push_generation !== 2 ||
    persisted.push_endpoint
  ) {
    throw new Error("MOBILE_INSTALLATION_LIST_INVARIANT");
  }

  return {
    identityID: persisted.identity_id,
    pushGeneration: persisted.push_generation,
    state: "REVOKED",
  };
}
