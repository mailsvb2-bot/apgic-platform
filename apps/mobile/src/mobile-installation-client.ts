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

export type InstallationE2EConfig = {
  baseURL: string;
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
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

type FetchLike = (url: string, options?: FetchOptions) => Promise<FetchResponse>;

const defaultFetch: FetchLike = (url, options) => fetch(url, options);

function requireInstallation(value: unknown): MobileInstallation {
  if (!value || typeof value !== "object") {
    throw new Error("MOBILE_INSTALLATION_RESPONSE_INVALID");
  }
  const candidate = value as Partial<MobileInstallation>;
  if (
    typeof candidate.id !== "string" ||
    typeof candidate.identity_id !== "string" ||
    (candidate.platform !== "IOS" && candidate.platform !== "ANDROID") ||
    typeof candidate.push_generation !== "number" ||
    (candidate.state !== "ACTIVE" && candidate.state !== "REVOKED")
  ) {
    throw new Error("MOBILE_INSTALLATION_RESPONSE_INVALID");
  }
  return candidate as MobileInstallation;
}

async function requestJSON(
  request: FetchLike,
  url: string,
  sessionCookie: string,
  method: string,
  expectedStatus: number,
  body?: Record<string, string>,
): Promise<unknown> {
  const response = await request(url, {
    method,
    headers: {
      Accept: "application/json",
      Cookie: sessionCookie,
      ...(body ? {"Content-Type": "application/json"} : {}),
    },
    ...(body ? {body: JSON.stringify(body)} : {}),
  });
  if (response.status !== expectedStatus) {
    throw new Error(`MOBILE_INSTALLATION_HTTP_${response.status}`);
  }
  return response.json();
}

export async function runInstallationE2ELifecycle(
  config: InstallationE2EConfig,
  request: FetchLike = defaultFetch,
): Promise<InstallationE2EResult> {
  if (typeof __DEV__ !== "undefined" && !__DEV__) {
    throw new Error("MOBILE_INSTALLATION_E2E_DISABLED");
  }
  const baseURL = config.baseURL.replace(/\/$/, "");
  if (
    !baseURL ||
    !config.sessionCookie ||
    !config.installationID ||
    (config.platform !== "IOS" && config.platform !== "ANDROID")
  ) {
    throw new Error("MOBILE_INSTALLATION_E2E_CONFIG_INVALID");
  }

  const tokenA = `e2e-${config.platform.toLowerCase()}-${config.installationID}-a`;
  const tokenB = `e2e-${config.platform.toLowerCase()}-${config.installationID}-b`;

  const registered = requireInstallation(
    await requestJSON(
      request,
      `${baseURL}/v1/mobile/installations`,
      config.sessionCookie,
      "POST",
      201,
      {
        id: config.installationID,
        platform: config.platform,
        push_endpoint: tokenA,
      },
    ),
  );
  if (
    registered.id !== config.installationID ||
    registered.platform !== config.platform ||
    registered.state !== "ACTIVE" ||
    registered.push_generation !== 1 ||
    registered.push_endpoint !== tokenA
  ) {
    throw new Error("MOBILE_INSTALLATION_REGISTER_INVARIANT");
  }

  const rotated = requireInstallation(
    await requestJSON(
      request,
      `${baseURL}/v1/mobile/installations/${config.installationID}/push-endpoint`,
      config.sessionCookie,
      "PATCH",
      200,
      {push_endpoint: tokenB},
    ),
  );
  if (
    rotated.identity_id !== registered.identity_id ||
    rotated.state !== "ACTIVE" ||
    rotated.push_generation !== 2 ||
    rotated.push_endpoint !== tokenB
  ) {
    throw new Error("MOBILE_INSTALLATION_ROTATE_INVARIANT");
  }

  const revoked = requireInstallation(
    await requestJSON(
      request,
      `${baseURL}/v1/mobile/installations/${config.installationID}/revoke`,
      config.sessionCookie,
      "POST",
      200,
    ),
  );
  if (
    revoked.identity_id !== registered.identity_id ||
    revoked.state !== "REVOKED" ||
    revoked.push_generation !== 2 ||
    revoked.push_endpoint
  ) {
    throw new Error("MOBILE_INSTALLATION_REVOKE_INVARIANT");
  }

  const listedPayload = await requestJSON(
    request,
    `${baseURL}/v1/mobile/installations`,
    config.sessionCookie,
    "GET",
    200,
  );
  if (!listedPayload || typeof listedPayload !== "object") {
    throw new Error("MOBILE_INSTALLATION_LIST_INVALID");
  }
  const listed = (listedPayload as {installations?: unknown}).installations;
  if (!Array.isArray(listed)) {
    throw new Error("MOBILE_INSTALLATION_LIST_INVALID");
  }
  const persisted = listed
    .map(requireInstallation)
    .find((candidate) => candidate.id === config.installationID);
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
