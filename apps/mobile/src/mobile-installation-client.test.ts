import assert from "node:assert/strict";
import test from "node:test";

import {
  registerMobileInstallation,
  runInstallationE2ELifecycle,
} from "./mobile-installation-client.ts";

test("installed client lifecycle preserves identity through rotation and revoke", async () => {
  const identityID = "11111111-1111-4111-8111-111111111111";
  const installationID = "22222222-2222-4222-8222-222222222222";
  const calls: Array<{url: string; method?: string; cookie?: string; body?: string}> = [];
  const responses = [
    {
      id: installationID,
      identity_id: identityID,
      platform: "IOS",
      push_endpoint: `e2e-ios-${installationID}-a`,
      push_generation: 1,
      state: "ACTIVE",
    },
    {
      id: installationID,
      identity_id: identityID,
      platform: "IOS",
      push_endpoint: `e2e-ios-${installationID}-b`,
      push_generation: 2,
      state: "ACTIVE",
    },
    {
      id: installationID,
      identity_id: identityID,
      platform: "IOS",
      push_generation: 2,
      state: "REVOKED",
    },
    {
      installations: [
        {
          id: installationID,
          identity_id: identityID,
          platform: "IOS",
          push_generation: 2,
          state: "REVOKED",
        },
      ],
    },
  ];
  let index = 0;
  const request = async (
    url: string,
    options?: {method?: string; headers?: Record<string, string>; body?: string},
  ) => {
    calls.push({
      url,
      method: options?.method,
      cookie: options?.headers?.Cookie,
      body: options?.body,
    });
    const body = responses[index++];
    return {
      status: index === 1 ? 201 : 200,
      async json() {
        return body;
      },
    };
  };

  const result = await runInstallationE2ELifecycle(
    {
      baseURL: "http://127.0.0.1:43113/",
      sessionCookie: "__Host-apgic_session=signed",
      installationID,
      platform: "IOS",
    },
    request,
  );

  assert.deepEqual(result, {
    identityID,
    pushGeneration: 2,
    state: "REVOKED",
  });
  assert.deepEqual(
    calls.map((call) => call.method),
    ["POST", "PATCH", "POST", "GET"],
  );
  assert.ok(calls.every((call) => call.cookie === "__Host-apgic_session=signed"));
  assert.match(calls[0].body ?? "", /"platform":"IOS"/);
});

test("lifecycle fails closed when rotation changes canonical identity", async () => {
  const installationID = "33333333-3333-4333-8333-333333333333";
  let call = 0;
  const request = async (_url: string) => {
    call += 1;
    const initial = {
      id: installationID,
      identity_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      platform: "ANDROID",
      push_endpoint: `e2e-android-${installationID}-a`,
      push_generation: 1,
      state: "ACTIVE",
    };
    const rotated = {
      ...initial,
      identity_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      push_endpoint: `e2e-android-${installationID}-b`,
      push_generation: 2,
    };
    return {
      status: call === 1 ? 201 : 200,
      async json() {
        return call === 1 ? initial : rotated;
      },
    };
  };

  await assert.rejects(
    runInstallationE2ELifecycle(
      {
        baseURL: "http://127.0.0.1:43113",
        sessionCookie: "__Host-apgic_session=signed",
        installationID,
        platform: "ANDROID",
      },
      request,
    ),
    /MOBILE_INSTALLATION_ROTATE_INVARIANT/,
  );
});


test("production registration uses the canonical authenticated HTTP contract", async () => {
  const installationID = "44444444-4444-4444-8444-444444444444";
  let captured:
    | {url: string; method?: string; cookie?: string; credentials?: string; body?: string}
    | undefined;
  const result = await registerMobileInstallation(
    {baseURL: "https://apgic.ru"},
    {
      id: installationID,
      platform: "ANDROID",
      pushEndpoint: "provider-token-1",
    },
    async (url, options) => {
      captured = {
        url,
        method: options?.method,
        cookie: options?.headers?.Cookie,
        credentials: options?.credentials,
        body: options?.body,
      };
      return {
        status: 201,
        async json() {
          return {
            id: installationID,
            identity_id: "55555555-5555-4555-8555-555555555555",
            platform: "ANDROID",
            push_endpoint: "provider-token-1",
            push_generation: 1,
            state: "ACTIVE",
          };
        },
      };
    },
  );

  assert.equal(result.idempotent, false);
  assert.equal(result.installation.id, installationID);
  assert.deepEqual(captured, {
    url: "https://apgic.ru/v1/mobile/installations",
    method: "POST",
    cookie: undefined,
    credentials: "include",
    body: JSON.stringify({
      id: installationID,
      platform: "ANDROID",
      push_endpoint: "provider-token-1",
    }),
  });
});

test("production client rejects insecure non-loopback API origins", async () => {
  await assert.rejects(
    registerMobileInstallation(
      {baseURL: "http://example.com"},
      {
        id: "66666666-6666-4666-8666-666666666666",
        platform: "IOS",
        pushEndpoint: "provider-token-2",
      },
      async () => {
        throw new Error("request must not be reached");
      },
    ),
    /MOBILE_INSTALLATION_BASE_URL_INVALID/,
  );
});

test("current-device revoke purges local user state after successful server revoke", async () => {
  const calls: string[] = [];
  const storage = {
    async saveCredential(_value: string) {},
    async loadCredential() { return null; },
    async clearUserScopedState() { calls.push("purge"); },
  };
  const request = async () =>
    new Response(
      JSON.stringify({
        id: "11111111-1111-4111-8111-111111111111",
        identity_id: "22222222-2222-4222-8222-222222222222",
        platform: "IOS",
        state: "REVOKED",
        push_generation: 2,
      }),
      {status: 200, headers: {"content-type": "application/json"}},
    );
  const {revokeCurrentMobileInstallation} = await import("./mobile-installation-client.ts");
  const result = await revokeCurrentMobileInstallation(
    {
      baseURL: "http://127.0.0.1:43111",
      sessionCookie: "session=test",
    },
    "11111111-1111-4111-8111-111111111111",
    storage,
    request,
  );
  assert.equal(result.state, "REVOKED");
  assert.deepEqual(calls, ["purge"]);
});

test("current-device revoke purges local user state even when server revoke fails", async () => {
  const calls: string[] = [];
  const storage = {
    async saveCredential(_value: string) {},
    async loadCredential() { return "stale"; },
    async clearUserScopedState() { calls.push("purge"); },
  };
  const request = async () => new Response("unavailable", {status: 503});
  const {revokeCurrentMobileInstallation} = await import("./mobile-installation-client.ts");
  await assert.rejects(
    revokeCurrentMobileInstallation(
      {
        baseURL: "http://127.0.0.1:43111",
        sessionCookie: "session=test",
      },
      "11111111-1111-4111-8111-111111111111",
      storage,
      request,
    ),
  );
  assert.deepEqual(calls, ["purge"]);
});

test("logout clears user-scoped local state", async () => {
  let purged = 0;
  const {logoutLocalSession} = await import("./mobile-installation-client.ts");
  await logoutLocalSession({
    async clearUserScopedState() {
      purged += 1;
    },
  });
  assert.equal(purged, 1);
});
