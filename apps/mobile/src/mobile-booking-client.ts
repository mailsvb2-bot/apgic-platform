import type { BookingStatus, OrderStatus } from "../../../packages/contracts/src/generated/apgic-v1";

type BookingFetch = (url: string, init?: {
  credentials?: "include";
  headers?: Record<string, string>;
  cache?: "no-store";
}) => Promise<{ status: number; json(): Promise<unknown> }>;

const knownStates = new Set<BookingStatus["state"]>([
  "HELD", "PENDING_PAYMENT", "CONFIRMED", "CANCELLED", "EXPIRED", "COMPLETED", "NO_SHOW",
]);

export function requireOwnedBookingStatus(payload: unknown, bookingID: string): BookingStatus {
  if (!payload || typeof payload !== "object") {
    throw new Error("BOOKING_STATUS_INVALID");
  }
  const view = payload as Partial<BookingStatus>;
  if (view.booking_id !== bookingID || !knownStates.has(view.state as BookingStatus["state"]) ||
    typeof view.hold_id !== "string" || !view.hold_id.trim() ||
    typeof view.hold_expires_at !== "string" || !Number.isFinite(Date.parse(view.hold_expires_at)) ||
    (view.starts_at !== undefined && (typeof view.starts_at !== "string" || !Number.isFinite(Date.parse(view.starts_at)))) ||
    (view.ends_at !== undefined && (typeof view.ends_at !== "string" || !Number.isFinite(Date.parse(view.ends_at)))) ||
    ((view.state === "CONFIRMED" || view.state === "COMPLETED") &&
      (view.starts_at === undefined || view.ends_at === undefined))) {
    throw new Error("BOOKING_STATUS_INVALID");
  }
  return view as BookingStatus;
}

export type BookingClientConfig = {
  baseURL: string;
  sessionCookie?: string;
};

// Native and web use one APGIC owner-scoped projection. Never infer confirmed
// booking, entitlement or payment outcome from cached device state.
export async function readOwnedBookingStatus(
  config: BookingClientConfig,
  bookingID: string,
  request: BookingFetch = (url, init) => fetch(url, init),
): Promise<BookingStatus> {
  const {baseURL, sessionCookie} = config;
  if (!bookingID.trim() || bookingID.includes("/") || bookingID.includes("?") || bookingID.includes("#")) {
    throw new Error("BOOKING_ID_INVALID");
  }
  let origin: URL;
  try { origin = new URL(baseURL); } catch { throw new Error("BOOKING_ORIGIN_INVALID"); }
  const local = origin.protocol === "http:" && (origin.hostname === "localhost" || origin.hostname === "127.0.0.1");
  if ((origin.protocol !== "https:" && !local) || origin.username || origin.password ||
      origin.search || origin.hash || (origin.pathname && origin.pathname !== "/")) {
    throw new Error("BOOKING_ORIGIN_INVALID");
  }
  const response = await request(origin.origin + "/v1/bookings/" + encodeURIComponent(bookingID), {
    credentials: "include",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(sessionCookie ? {Cookie: sessionCookie} : {}),
    },
  });
  if (response.status === 401 || response.status === 403) throw new Error("BOOKING_SESSION_REQUIRED");
  if (response.status === 404) throw new Error("BOOKING_NOT_FOUND");
  if (response.status !== 200) throw new Error("BOOKING_OWNER_UNAVAILABLE");
  return requireOwnedBookingStatus(await response.json(), bookingID);
}

const knownOrderPayments = new Set<OrderStatus["payment_state"]>(["PENDING_PROVIDER", "CAPTURED"]);
const knownRefunds = new Set<NonNullable<OrderStatus["refund_state"]>>([
  "REQUESTED", "SENT", "PENDING", "SUCCEEDED", "FAILED_TERMINAL", "AMBIGUOUS",
]);

export function requireOwnedOrderStatus(payload: unknown, orderID: string): OrderStatus {
  if (!payload || typeof payload !== "object") {
    throw new Error("ORDER_STATUS_INVALID");
  }
  const value = payload as Partial<OrderStatus>;
  if (value.order_id !== orderID ||
      typeof value.booking_id !== "string" || !value.booking_id.trim() ||
      !knownStates.has(value.booking_state as BookingStatus["state"]) ||
      !knownOrderPayments.has(value.payment_state as OrderStatus["payment_state"]) ||
      typeof value.amount_minor !== "number" || !Number.isSafeInteger(value.amount_minor) ||
      value.amount_minor <= 0 ||
      typeof value.currency !== "string" || !/^[A-Z]{3}$/.test(value.currency) ||
      typeof value.provider_id !== "string" || !value.provider_id.trim() ||
      typeof value.payment_method !== "string" || !value.payment_method.trim() ||
      (value.refund_state !== undefined && !knownRefunds.has(value.refund_state)) ||
      (value.refund_state !== undefined && value.payment_state !== "CAPTURED") ||
      (value.payment_state === "CAPTURED" &&
       (value.booking_state === "HELD" || value.booking_state === "PENDING_PAYMENT"))) {
    throw new Error("ORDER_STATUS_INVALID");
  }
  return value as OrderStatus;
}

// Refreshes server-owned financial and booking evidence on the same secure
// API origin as native booking recovery. Neither successful refund nor capture
// can be derived from a local booking state.
export async function readOwnedOrderStatus(
  config: BookingClientConfig,
  orderID: string,
  request: BookingFetch = (url, init) => fetch(url, init),
): Promise<OrderStatus> {
  if (!orderID.trim() || orderID.includes("/") || orderID.includes("?") || orderID.includes("#")) {
    throw new Error("ORDER_ID_INVALID");
  }
  const {baseURL, sessionCookie} = config;
  let origin: URL;
  try { origin = new URL(baseURL); } catch { throw new Error("ORDER_ORIGIN_INVALID"); }
  const local = origin.protocol === "http:" && (origin.hostname === "localhost" || origin.hostname === "127.0.0.1");
  if ((origin.protocol !== "https:" && !local) || origin.username || origin.password ||
      origin.search || origin.hash || (origin.pathname && origin.pathname !== "/")) {
    throw new Error("ORDER_ORIGIN_INVALID");
  }
  const response = await request(origin.origin + "/v1/orders/" + encodeURIComponent(orderID), {
    credentials: "include",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(sessionCookie ? {Cookie: sessionCookie} : {}),
    },
  });
  if (response.status === 401 || response.status === 403) throw new Error("ORDER_SESSION_REQUIRED");
  if (response.status === 404) throw new Error("ORDER_NOT_FOUND");
  if (response.status !== 200) throw new Error("ORDER_OWNER_UNAVAILABLE");
  return requireOwnedOrderStatus(await response.json(), orderID);
}

/**
 * Deep-linked native booking pages restore the canonical payment without
 * relying on an order ID cached on the device. A missing checkout is a 404,
 * not evidence of a completed, failed or refunded payment.
 */
export async function readOwnedOrderForBooking(
  config: BookingClientConfig,
  bookingID: string,
  request: BookingFetch = (url, init) => fetch(url, init),
): Promise<OrderStatus> {
  if (!bookingID.trim() || bookingID.includes("/") || bookingID.includes("?") || bookingID.includes("#")) {
    throw new Error("BOOKING_ID_INVALID");
  }
  const {baseURL, sessionCookie} = config;
  let origin: URL;
  try { origin = new URL(baseURL); } catch { throw new Error("ORDER_ORIGIN_INVALID"); }
  const local = origin.protocol === "http:" && (origin.hostname === "localhost" || origin.hostname === "127.0.0.1");
  if ((origin.protocol !== "https:" && !local) || origin.username || origin.password ||
      origin.search || origin.hash || (origin.pathname && origin.pathname !== "/")) {
    throw new Error("ORDER_ORIGIN_INVALID");
  }
  const response = await request(origin.origin + "/v1/bookings/" + encodeURIComponent(bookingID) + "/order", {
    credentials: "include",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(sessionCookie ? {Cookie: sessionCookie} : {}),
    },
  });
  if (response.status === 401 || response.status === 403) throw new Error("ORDER_SESSION_REQUIRED");
  if (response.status === 404) throw new Error("ORDER_NOT_FOUND");
  if (response.status !== 200) throw new Error("ORDER_OWNER_UNAVAILABLE");
  const payload = await response.json();
  if (!payload || typeof payload !== "object" ||
      (payload as Partial<OrderStatus>).booking_id !== bookingID ||
      typeof (payload as Partial<OrderStatus>).order_id !== "string") {
    throw new Error("ORDER_STATUS_INVALID");
  }
  return requireOwnedOrderStatus(payload, (payload as OrderStatus).order_id);
}
