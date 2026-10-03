import assert from "node:assert/strict";
import test from "node:test";

import {
  listAuthorizedWorkspaces,
  resolveAuthorizedWorkspace,
  runWorkspaceE2EFlow,
} from "./mobile-workspace-client.ts";

const own = [
  {
    workspace_id: "client:identity-1",
    identity_id: "identity-1",
    tenant_id: "identity-1",
    kind: "CLIENT" as const,
    authorization_decision: "ALLOW" as const,
    reason_code: "WORKSPACE_ALLOWED",
  },
  {
    workspace_id: "specialist:specialist-1",
    identity_id: "identity-1",
    tenant_id: "identity-1",
    kind: "SPECIALIST" as const,
    authorization_decision: "ALLOW" as const,
    reason_code: "WORKSPACE_ALLOWED",
  },
  {
    workspace_id: "organization:organization-1",
    identity_id: "identity-1",
    tenant_id: "organization-1",
    kind: "ORGANIZATION" as const,
    authorization_decision: "ALLOW" as const,
    reason_code: "WORKSPACE_ALLOWED",
  },
];

test("workspace client accepts only server-authorized canonical projections", async () => {
  const workspaces = await listAuthorizedWorkspaces({
    baseURL: "http://127.0.0.1:43113",
    sessionCookie: "__Host-apgic_session=signed",
    request: async (url, options) => {
      assert.equal(url, "http://127.0.0.1:43113/v1/mobile/workspaces");
      assert.equal(options?.headers?.Cookie, "__Host-apgic_session=signed");
      return {
        status: 200,
        async json() {
          return {workspaces: own};
        },
      };
    },
  });
  assert.deepEqual(workspaces, own);
});

test("workspace resolution fails closed on DENY and malformed ALLOW", async () => {
  assert.equal(
    await resolveAuthorizedWorkspace("organization:foreign", {
      baseURL: "http://127.0.0.1:43113",
      request: async () => ({
        status: 200,
        async json() {
          return {
            allowed: false,
            reason_code: "WORKSPACE_AUTHORIZATION_DENY",
          };
        },
      }),
    }),
    null,
  );

  assert.equal(
    await resolveAuthorizedWorkspace("client:identity-1", {
      baseURL: "http://127.0.0.1:43113",
      request: async () => ({
        status: 200,
        async json() {
          return {
            allowed: true,
            reason_code: "WORKSPACE_ALLOWED",
            workspace: {
              ...own[0],
              authorization_decision: "DENY",
            },
          };
        },
      }),
    }),
    null,
  );
});

test("workspace E2E switches three roles on one identity and denies foreign scope", async () => {
  const request = async (url: string) => {
    if (url.endsWith("/v1/mobile/workspaces")) {
      return {
        status: 200,
        async json() {
          return {workspaces: own};
        },
      };
    }
    const encoded = url.split("/").at(-1) ?? "";
    const id = decodeURIComponent(encoded);
    const found = own.find((workspace) => workspace.workspace_id === id);
    return {
      status: 200,
      async json() {
        return found
          ? {
              allowed: true,
              reason_code: "WORKSPACE_ALLOWED",
              workspace: found,
            }
          : {
              allowed: false,
              reason_code: "WORKSPACE_AUTHORIZATION_DENY",
            };
      },
    };
  };

  const result = await runWorkspaceE2EFlow({
    baseURL: "http://127.0.0.1:43113",
    sessionCookie: "__Host-apgic_session=signed",
    request,
  });
  assert.equal(result.identityID, "identity-1");
  assert.deepEqual(result.workspaceKinds, [
    "CLIENT",
    "SPECIALIST",
    "ORGANIZATION",
  ]);
  assert.equal(result.foreignWorkspaceDenied, true);
});

test("workspace client rejects alternate API origin before network", async () => {
  let called = false;
  await assert.rejects(
    listAuthorizedWorkspaces({
      baseURL: "https://evil.example",
      request: async () => {
        called = true;
        throw new Error("must not call");
      },
    }),
    /WORKSPACE_API_ORIGIN_INVALID/,
  );
  assert.equal(called, false);
});
