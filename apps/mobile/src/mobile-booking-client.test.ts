import assert from "node:assert/strict";
import test from "node:test";
import { readOwnedBookingStatus, readOwnedOrderStatus } from "./mobile-booking-client.ts";

const data = {
  booking_id: "booking-1", hold_id: "hold-1", state: "CONFIRMED",
  hold_expires_at: "2026-10-12T10:00:00Z",
  starts_at: "2026-10-15T10:00:00Z", ends_at: "2026-10-15T11:00:00Z",
};
const request = (payload: unknown, status = 200) => async () => ({
  status, async json() { return payload; },
});

test("native booking resume reads canonical owner and does not guess payment", async () => {
  const actual = await readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "booking-1", request(data));
  assert.equal(actual.state, "CONFIRMED");
  let forwardedCookie = "";
  await readOwnedBookingStatus(
    {baseURL: "https://apgic.ru", sessionCookie: "__Host-apgic_session=trusted"},
    "booking-1",
    async (_url, options) => {
      forwardedCookie = options?.headers?.Cookie ?? "";
      return {status: 200, async json() { return data; }};
    },
  );
  assert.equal(forwardedCookie, "__Host-apgic_session=trusted");
  await assert.rejects(
    readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "booking-1", request({...data, booking_id: "foreign"})),
    /BOOKING_STATUS_INVALID/,
  );
  await assert.rejects(
    readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "booking-1", request({...data, state: "UNVERIFIED"})),
    /BOOKING_STATUS_INVALID/,
  );
});

test("native booking resume fails closed when session, owner or server is unavailable", async () => {
  for (const [status, expected] of [
    [401, /BOOKING_SESSION_REQUIRED/], [403, /BOOKING_SESSION_REQUIRED/],
    [404, /BOOKING_NOT_FOUND/], [503, /BOOKING_OWNER_UNAVAILABLE/],
  ] as const) {
    await assert.rejects(readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "booking-1", request({}, status)), expected);
  }
  await assert.rejects(readOwnedBookingStatus({baseURL: "http://evil.example"}, "booking-1", request(data)), /BOOKING_ORIGIN_INVALID/);
  await assert.rejects(readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "../foreign", request(data)), /BOOKING_ID_INVALID/);
  await assert.rejects(readOwnedBookingStatus({baseURL: "https://apgic.ru"}, "booking-1", request({...data, state:"COMPLETED", starts_at: undefined})), /BOOKING_STATUS_INVALID/);
});

const order = {
  order_id: "order-1", booking_id: "booking-1",
  booking_state: "PENDING_PAYMENT", payment_state: "PENDING_PROVIDER",
  amount_minor: 120000, currency: "RUB", provider_id: "bank-a",
  payment_method: "SBP",
};

test("native order resume reads verified payment evidence, never infers capture from booking", async () => {
  let observedURL = "";
  let observedCookie = "";
  const pending = await readOwnedOrderStatus(
    {baseURL: "https://apgic.ru", sessionCookie: "__Host-apgic_session=trusted"},
    "order-1",
    async (url, init) => {
      observedURL = url;
      observedCookie = init?.headers?.Cookie ?? "";
      assert.equal(init?.cache, "no-store");
      return {status: 200, async json() { return order; }};
    },
  );
  assert.equal(observedURL, "https://apgic.ru/v1/orders/order-1");
  assert.equal(observedCookie, "__Host-apgic_session=trusted");
  assert.equal(pending.payment_state, "PENDING_PROVIDER");
  const captured = {...order, booking_state: "CONFIRMED", payment_state: "CAPTURED"};
  assert.equal(
    (await readOwnedOrderStatus({baseURL: "https://apgic.ru"}, "order-1", request(captured))).payment_state,
    "CAPTURED",
  );
  const refunded = {...captured, booking_state: "CANCELLED", refund_state: "SUCCEEDED"};
  assert.equal(
    (await readOwnedOrderStatus({baseURL: "https://apgic.ru"}, "order-1", request(refunded))).refund_state,
    "SUCCEEDED",
  );
});

test("native order resume denies forged outcomes, foreign orders, unsafe origin, and outages", async () => {
  for (const bad of [
    {...order, order_id: "foreign"},
    {...order, payment_state: "CAPTURED"},
    {...order, refund_state: "SUCCEEDED"},
    {...order, amount_minor: -1},
    {...order, currency: "rub"},
    {...order, payment_method: ""},
    {...order, booking_state: "DENIED"},
    {...order, provider_id: ""},
    {...order, refund_state: "FREE"},
  ]) {
    await assert.rejects(
      readOwnedOrderStatus({baseURL: "https://apgic.ru"}, "order-1", request(bad)),
      /ORDER_STATUS_INVALID/,
    );
  }
  for (const [status, code] of [
    [401, /ORDER_SESSION_REQUIRED/], [403, /ORDER_SESSION_REQUIRED/],
    [404, /ORDER_NOT_FOUND/], [503, /ORDER_OWNER_UNAVAILABLE/],
  ] as const) {
    await assert.rejects(readOwnedOrderStatus({baseURL: "https://apgic.ru"}, "order-1", request({}, status)), code);
  }
  await assert.rejects(readOwnedOrderStatus({baseURL: "http://attacker.example"}, "order-1", request(order)), /ORDER_ORIGIN_INVALID/);
  await assert.rejects(readOwnedOrderStatus({baseURL: "https://apgic.ru"}, "../foreign", request(order)), /ORDER_ID_INVALID/);
});
