import type { BookingStatus } from "../../../packages/contracts/src/generated/apgic-v1";

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

// Native and web use one APGIC owner-scoped projection. Never infer confirmed
// booking, entitlement or payment outcome from cached device state.
export async function readOwnedBookingStatus(
  baseURL: string,
  bookingID: string,
  request: BookingFetch = (url, init) => fetch(url, init),
): Promise<BookingStatus> {
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
    headers: { Accept: "application/json" },
  });
  if (response.status === 401 || response.status === 403) throw new Error("BOOKING_SESSION_REQUIRED");
  if (response.status === 404) throw new Error("BOOKING_NOT_FOUND");
  if (response.status !== 200) throw new Error("BOOKING_OWNER_UNAVAILABLE");
  return requireOwnedBookingStatus(await response.json(), bookingID);
}
