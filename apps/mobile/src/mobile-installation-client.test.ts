import assert from "node:assert/strict";
import test from "node:test";

import {runInstallationE2ELifecycle} from "./mobile-installation-client.ts";

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
