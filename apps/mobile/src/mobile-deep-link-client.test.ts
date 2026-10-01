import assert from "node:assert/strict";
import test from "node:test";

import {
  extractCanonicalDeepLinkToken,
  resolveCanonicalUniversalLink,
} from "./mobile-deep-link-client.ts";

const token = "v1.c2VhbGVkLWFlYWQtYmxvYg";

test("canonical universal-link parser rejects alternate origins and URL smuggling", () => {
  assert.equal(extractCanonicalDeepLinkToken(`https://apgic.ru/l/${token}`), token);
  for (const value of [
    `http://apgic.ru/l/${token}`,
    `https://evil.example/l/${token}`,
    `https://apgic.ru.evil.example/l/${token}`,
    `https://user@apgic.ru/l/${token}`,
    `https://apgic.ru/l/${token}?next=https://evil.example`,
    `https://apgic.ru/l/${token}#fragment`,
    "https://apgic.ru/l/not-a-signed-token",
    `https://apgic.ru/l/v1.${"A".repeat(4097)}`,
  ]) {
    assert.equal(extractCanonicalDeepLinkToken(value), null, value);
  }
});

test("native client only opens canonical path after server ALLOW", async () => {
  const action = await resolveCanonicalUniversalLink(
    `https://apgic.ru/l/${token}`,
    {
      apiOrigin: "http://127.0.0.1:43114",
      sessionCookie: "__Host-apgic_session=signed",
      request: async (url, options) => {
        assert.match(url, /\/v1\/mobile\/deep-links\/resolve\?token=/);
        assert.equal(options?.headers?.Cookie, "__Host-apgic_session=signed");
        return {
          status: 200,
          async json() {
            return {
              decision: "ALLOW",
              reason_code: "DEEPLINK_ALLOWED",
              canonical_path: "/bookings/booking-1",
              canonical_web_fallback: "https://apgic.ru/bookings/booking-1",
              expires_at: "2026-10-01T12:15:00Z",
            };
          },
        };
      },
    },
  );
  assert.deepEqual(action, {action: "OPEN_APP_PATH", target: "/bookings/booking-1"});
});

test("native client fails closed on invalid or denied server responses", async () => {
  assert.deepEqual(
    await resolveCanonicalUniversalLink("https://evil.example/l/" + token),
    {action: "BLOCK"},
  );
  const denied = await resolveCanonicalUniversalLink(
    `https://apgic.ru/l/${token}`,
    {
      apiOrigin: "http://127.0.0.1:43114",
      request: async () => ({
        status: 200,
        async json() {
          return {decision: "DENY", reason_code: "DEEPLINK_EXPIRED"};
        },
      }),
    },
  );
  assert.deepEqual(denied, {action: "BLOCK"});
});


test("native client rejects alternate API origins and malformed ALLOW payloads", async () => {
  let called = false;
  assert.deepEqual(
    await resolveCanonicalUniversalLink(`https://apgic.ru/l/${token}`, {
      apiOrigin: "https://evil.example",
      request: async () => {
        called = true;
        throw new Error("must not call alternate origin");
      },
    }),
    {action: "BLOCK"},
  );
  assert.equal(called, false);

  for (const payload of [
    {
      decision: "DENY",
      reason_code: "DEEPLINK_AUTHORIZATION_DENY",
      canonical_web_fallback: "https://apgic.ru/bookings/booking-1",
    },
    {
      decision: "ALLOW",
      reason_code: "DEEPLINK_ALLOWED",
      canonical_path: "//evil.example",
      canonical_web_fallback: "https://evil.example/",
      expires_at: "2026-10-01T12:15:00Z",
    },
    {
      decision: "ALLOW",
      reason_code: "DEEPLINK_ALLOWED",
      canonical_path: "/bookings/booking-1",
      canonical_web_fallback: "https://apgic.ru/bookings/other-booking",
      expires_at: "2026-10-01T12:15:00Z",
    },
  ]) {
    const action = await resolveCanonicalUniversalLink(
      `https://apgic.ru/l/${token}`,
      {
        apiOrigin: "http://127.0.0.1:43114",
        request: async () => ({
          status: 200,
          async json() {
            return payload;
          },
        }),
      },
    );
    assert.deepEqual(action, {action: "BLOCK"});
  }
});
