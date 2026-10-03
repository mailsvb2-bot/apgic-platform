import type {
  AuthorizedWorkspaceV1,
  WorkspaceKind,
  WorkspaceListV1,
  WorkspaceResolutionV1,
} from "../../../packages/contracts/src/r1-mobile";
import {consumeAuthorizedWorkspace} from "./r1-cross-surface.ts";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

type FetchLike = (url: string, options?: FetchOptions) => Promise<FetchResponse>;

const defaultFetch: FetchLike = (url, options) => fetch(url, options);
const workspaceKinds = new Set<WorkspaceKind>([
  "CLIENT",
  "SPECIALIST",
  "ORGANIZATION",
]);

function allowedAPIOrigin(raw: string): string | null {
  const origin = raw.replace(/\/$/, "");
  if (
    origin === canonicalAPGICOrigin ||
    /^http:\/\/127\.0\.0\.1(?::\d+)?$/.test(origin)
  ) {
    return origin;
  }
  return null;
}

function parseAuthorizedWorkspace(value: unknown): AuthorizedWorkspaceV1 | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const candidate = value as Partial<AuthorizedWorkspaceV1>;
  if (
    typeof candidate.workspace_id !== "string" ||
    candidate.workspace_id.length === 0 ||
    candidate.workspace_id.length > 256 ||
    typeof candidate.identity_id !== "string" ||
    candidate.identity_id.length === 0 ||
    typeof candidate.tenant_id !== "string" ||
    candidate.tenant_id.length === 0 ||
    !workspaceKinds.has(candidate.kind as WorkspaceKind) ||
    candidate.authorization_decision !== "ALLOW" ||
    candidate.reason_code !== "WORKSPACE_ALLOWED"
  ) {
    return null;
  }
  const prefix =
    candidate.kind === "CLIENT"
      ? "client:"
      : candidate.kind === "SPECIALIST"
        ? "specialist:"
        : "organization:";
  if (!candidate.workspace_id.startsWith(prefix)) {
    return null;
  }
  return candidate as AuthorizedWorkspaceV1;
}

function parseWorkspaceList(value: unknown): WorkspaceListV1 | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const candidate = value as Partial<WorkspaceListV1>;
  if (!Array.isArray(candidate.workspaces)) {
    return null;
  }
  const workspaces = candidate.workspaces
    .map(parseAuthorizedWorkspace)
    .filter((workspace): workspace is AuthorizedWorkspaceV1 => workspace !== null);
  if (workspaces.length !== candidate.workspaces.length) {
    return null;
  }
  const ids = new Set(workspaces.map((workspace) => workspace.workspace_id));
  if (ids.size !== workspaces.length) {
    return null;
  }
  return {workspaces};
}

function parseWorkspaceResolution(value: unknown): WorkspaceResolutionV1 | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const candidate = value as Partial<WorkspaceResolutionV1>;
  if (
    typeof candidate.allowed !== "boolean" ||
    typeof candidate.reason_code !== "string" ||
    candidate.reason_code.length === 0
  ) {
    return null;
  }
  if (!candidate.allowed) {
    if (candidate.workspace !== undefined) {
      return null;
    }
    return {
      allowed: false,
      reason_code: candidate.reason_code,
    };
  }
  const workspace = parseAuthorizedWorkspace(candidate.workspace);
  if (!workspace || candidate.reason_code !== "WORKSPACE_ALLOWED") {
    return null;
  }
  return {
    allowed: true,
    reason_code: candidate.reason_code,
    workspace,
  };
}

function requestOptions(sessionCookie?: string): FetchOptions {
  const headers: Record<string, string> = {Accept: "application/json"};
  if (sessionCookie) {
    headers.Cookie = sessionCookie;
  }
  return {method: "GET", headers, credentials: "include"};
}

export async function listAuthorizedWorkspaces(options: {
  baseURL?: string;
  sessionCookie?: string;
  request?: FetchLike;
}): Promise<AuthorizedWorkspaceV1[]> {
  const baseURL = allowedAPIOrigin(options.baseURL ?? canonicalAPGICOrigin);
  if (!baseURL) {
    throw new Error("WORKSPACE_API_ORIGIN_INVALID");
  }
  const response = await (options.request ?? defaultFetch)(
    `${baseURL}/v1/mobile/workspaces`,
    requestOptions(options.sessionCookie),
  );
  if (response.status !== 200) {
    throw new Error(`WORKSPACE_LIST_HTTP_${response.status}`);
  }
  const parsed = parseWorkspaceList(await response.json());
  if (!parsed) {
    throw new Error("WORKSPACE_LIST_INVALID");
  }
  return parsed.workspaces;
}

export async function resolveAuthorizedWorkspace(
  workspaceID: string,
  options: {
    baseURL?: string;
    sessionCookie?: string;
    request?: FetchLike;
  },
): Promise<AuthorizedWorkspaceV1 | null> {
  const baseURL = allowedAPIOrigin(options.baseURL ?? canonicalAPGICOrigin);
  const normalizedID = workspaceID.trim();
  if (!baseURL || normalizedID.length === 0 || normalizedID.length > 256) {
    return null;
  }
  const response = await (options.request ?? defaultFetch)(
    `${baseURL}/v1/mobile/workspaces/${encodeURIComponent(normalizedID)}`,
    requestOptions(options.sessionCookie),
  );
  if (response.status !== 200) {
    return null;
  }
  const parsed = parseWorkspaceResolution(await response.json());
  return parsed ? consumeAuthorizedWorkspace(parsed) : null;
}

export type WorkspaceE2EResult = {
  identityID: string;
  workspaceKinds: WorkspaceKind[];
  foreignWorkspaceDenied: boolean;
};

export async function runWorkspaceE2EFlow(options: {
  baseURL: string;
  sessionCookie: string;
  foreignWorkspaceID?: string;
  request?: FetchLike;
}): Promise<WorkspaceE2EResult> {
  const workspaces = await listAuthorizedWorkspaces(options);
  const expectedKinds: WorkspaceKind[] = [
    "CLIENT",
    "SPECIALIST",
    "ORGANIZATION",
  ];
  const byKind = new Map(workspaces.map((workspace) => [workspace.kind, workspace]));
  for (const kind of expectedKinds) {
    if (!byKind.has(kind)) {
      throw new Error(`WORKSPACE_E2E_MISSING_${kind}`);
    }
  }
  const identityID = byKind.get("CLIENT")!.identity_id;
  if (
    expectedKinds.some((kind) => byKind.get(kind)!.identity_id !== identityID)
  ) {
    throw new Error("WORKSPACE_E2E_IDENTITY_SPLIT");
  }

  for (const kind of expectedKinds) {
    const expected = byKind.get(kind)!;
    const resolved = await resolveAuthorizedWorkspace(expected.workspace_id, options);
    if (
      !resolved ||
      resolved.workspace_id !== expected.workspace_id ||
      resolved.identity_id !== identityID ||
      resolved.kind !== kind
    ) {
      throw new Error(`WORKSPACE_E2E_SWITCH_${kind}_FAILED`);
    }
  }

  const foreignWorkspaceID =
    options.foreignWorkspaceID ?? "organization:00000000-0000-4000-8000-000000000099";
  const foreign = await resolveAuthorizedWorkspace(foreignWorkspaceID, options);
  if (foreign !== null) {
    throw new Error("WORKSPACE_E2E_FOREIGN_SCOPE_ALLOWED");
  }

  return {
    identityID,
    workspaceKinds: expectedKinds,
    foreignWorkspaceDenied: true,
  };
}
