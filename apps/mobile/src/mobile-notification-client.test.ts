import assert from "node:assert/strict";
import test from "node:test";

import {
  requireNotificationTransport,
  resolvePushNotification,
  type NotificationFetch,
} from "./mobile-notification-client.ts";

const transport = {
  contract_version: "notification-transport-v1" as const,
  delivery_id: "delivery-1",
  intent_id: "intent-1",
};

function response(payload: unknown, status = 200): NotificationFetch {
  return async () => ({status, async json() { return payload; }});
}

test("push transport envelope cannot smuggle raw notification content", () => {
  assert.throws(
    () => requireNotificationTransport({...transport, body: "private consultation details"}),
    /NOTIFICATION_TRANSPORT_INVALID/,
  );
});

test("sensitive push re-reads canonical generic projection from server", async () => {
  const projection = await resolvePushNotification(
    transport,
    {baseURL: "https://apgic.ru", sessionCookie: "__Host-apgic_session=signed"},
    response({
      contract_version: "notification-projection-v1",
      delivery_id: "delivery-1",
      intent_id: "intent-1",
      purpose: "BOOKING_CONFIRMATION",
      related_object_ref: "booking/booking-1",
      channel: "PUSH",
      delivery_state: "PENDING",
      data_class: "SENSITIVE",
      preview_mode: "GENERIC",
    }),
  );
  assert.equal(projection.intent_id, "intent-1");
  assert.equal(projection.preview_mode, "GENERIC");
});

test("push fails closed on scope mismatch or unsafe sensitive preview", async () => {
  await assert.rejects(
    resolvePushNotification(
      transport,
      {baseURL: "https://apgic.ru"},
      response({
        contract_version: "notification-projection-v1",
        delivery_id: "delivery-1",
        intent_id: "another-intent",
        purpose: "BOOKING_CONFIRMATION",
        related_object_ref: "booking/booking-1",
        channel: "PUSH",
        delivery_state: "PENDING",
        data_class: "SENSITIVE",
        preview_mode: "GENERIC",
      }),
    ),
    /NOTIFICATION_TRANSPORT_SCOPE_MISMATCH/,
  );
  await assert.rejects(
    resolvePushNotification(
      transport,
      {baseURL: "https://apgic.ru"},
      response({
        contract_version: "notification-projection-v1",
        delivery_id: "delivery-1",
        intent_id: "intent-1",
        purpose: "BOOKING_CONFIRMATION",
        related_object_ref: "booking/booking-1",
        channel: "PUSH",
        delivery_state: "PENDING",
        data_class: "RAW_CONSULTATION",
        preview_mode: "FULL",
      }),
    ),
    /NOTIFICATION_SENSITIVE_PREVIEW_UNSAFE/,
  );
});
