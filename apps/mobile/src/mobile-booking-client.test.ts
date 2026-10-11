import assert from "node:assert/strict";
import test from "node:test";
import { readOwnedBookingStatus } from "./mobile-booking-client.ts";

const data = {
  booking_id: "booking-1", hold_id: "hold-1", state: "CONFIRMED",
  hold_expires_at: "2026-10-12T10:00:00Z",
  starts_at: "2026-10-15T10:00:00Z", ends_at: "2026-10-15T11:00:00Z",
};
const request = (payload: unknown, status = 200) => async () => ({
  status, async json() { return payload; },
});

test("native booking resume reads canonical owner and does not guess payment", async () => {
  const actual = await readOwnedBookingStatus("https://apgic.ru", "booking-1", request(data));
  assert.equal(actual.state, "CONFIRMED");
  await assert.rejects(
    readOwnedBookingStatus("https://apgic.ru", "booking-1", request({...data, booking_id: "foreign"})),
    /BOOKING_STATUS_INVALID/,
  );
  await assert.rejects(
    readOwnedBookingStatus("https://apgic.ru", "booking-1", request({...data, state: "UNVERIFIED"})),
    /BOOKING_STATUS_INVALID/,
  );
});

test("native booking resume fails closed when session, owner or server is unavailable", async () => {
  for (const [status, expected] of [
    [401, /BOOKING_SESSION_REQUIRED/], [403, /BOOKING_SESSION_REQUIRED/],
    [404, /BOOKING_NOT_FOUND/], [503, /BOOKING_OWNER_UNAVAILABLE/],
  ] as const) {
    await assert.rejects(readOwnedBookingStatus("https://apgic.ru", "booking-1", request({}, status)), expected);
  }
  await assert.rejects(readOwnedBookingStatus("http://evil.example", "booking-1", request(data)), /BOOKING_ORIGIN_INVALID/);
  await assert.rejects(readOwnedBookingStatus("https://apgic.ru", "../foreign", request(data)), /BOOKING_ID_INVALID/);
  await assert.rejects(readOwnedBookingStatus("https://apgic.ru", "booking-1", request({...data, state:"COMPLETED", starts_at: undefined})), /BOOKING_STATUS_INVALID/);
});
