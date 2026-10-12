// Shared presentation contract. APGIC PostgreSQL owns the canonical facts.
export type ClientPersonalPreferences = {
  display_name: string;
  locale: "ru" | "en";
  time_zone: string;
  updated_at?: string;
};
export type ClientPersonalBooking = {
  booking_id: string;
  state: "HELD" | "PENDING_PAYMENT" | "CONFIRMED" | "CANCELLED" | "EXPIRED" | "COMPLETED" | "NO_SHOW";
  starts_at: string;
  ends_at: string;
  specialist_name: string;
  specialist_profession: string;
  consultation_state?: "SCHEDULED" | "READY" | "IN_PROGRESS" | "RECOVERING" | "TECHNICAL_FAILURE" | "COMPLETED";
  order_id?: string;
};
export type ClientPersonalRequest = {
  id: string;
  state: "DRAFT" | "CONFIRMED";
  topics: string[];
  created_at: string;
};
export type ClientPersonalView = {
  identity_id: string;
  as_of: string;
  preferences: ClientPersonalPreferences;
  upcoming: ClientPersonalBooking[];
  history: ClientPersonalBooking[];
  requests: ClientPersonalRequest[];
  next_history_cursor?: string;
};

const states = new Set(["HELD","PENDING_PAYMENT","CONFIRMED","CANCELLED","EXPIRED","COMPLETED","NO_SHOW"]);
const consultStates = new Set(["SCHEDULED","READY","IN_PROGRESS","RECOVERING","TECHNICAL_FAILURE","COMPLETED"]);
function record(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error("PERSONAL_RESPONSE_INVALID");
  return value as Record<string, unknown>;
}
function validBooking(value: unknown): value is ClientPersonalBooking {
  if (value === null || typeof value !== "object") return false;
  const b = value as Partial<ClientPersonalBooking>;
  return typeof b.booking_id === "string" && b.booking_id.length === 36 &&
    typeof b.state === "string" && states.has(b.state) &&
    typeof b.specialist_name === "string" && typeof b.specialist_profession === "string" &&
    typeof b.starts_at === "string" && Number.isFinite(Date.parse(b.starts_at)) &&
    typeof b.ends_at === "string" && Number.isFinite(Date.parse(b.ends_at)) &&
    (b.consultation_state === undefined || consultStates.has(b.consultation_state)) &&
    (b.order_id === undefined || (typeof b.order_id === "string" && b.order_id.length === 36));
}
export function parseClientPersonalView(value: unknown): ClientPersonalView {
  const v = record(value);
  const p = record(v.preferences);
  if (typeof v.identity_id !== "string" || v.identity_id.length !== 36 ||
      typeof v.as_of !== "string" || !Number.isFinite(Date.parse(v.as_of)) ||
      typeof p.display_name !== "string" || (p.locale !== "ru" && p.locale !== "en") ||
      typeof p.time_zone !== "string" || !Array.isArray(v.upcoming) || !Array.isArray(v.history) ||
      !v.upcoming.every(validBooking) || !v.history.every(validBooking) ||
      !Array.isArray(v.requests) || !v.requests.every((r: unknown) => {
        if (!r || typeof r !== "object") return false;
        const x = r as Partial<ClientPersonalRequest>;
        return typeof x.id === "string" && x.id.length === 36 &&
          (x.state === "DRAFT" || x.state === "CONFIRMED") &&
          Array.isArray(x.topics) && x.topics.every((t) => typeof t === "string") &&
          typeof x.created_at === "string" && Number.isFinite(Date.parse(x.created_at));
      }) ||
      (v.next_history_cursor !== undefined && typeof v.next_history_cursor !== "string")) {
    throw new Error("PERSONAL_RESPONSE_INVALID");
  }
  return v as ClientPersonalView;
}
export const personalBookingLabel: Record<ClientPersonalBooking["state"], string> = {
  HELD: "Время удерживается", PENDING_PAYMENT: "Ожидает оплаты",
  CONFIRMED: "Подтверждена", CANCELLED: "Отменена",
  EXPIRED: "Время истекло", COMPLETED: "Завершена",
  NO_SHOW: "Не состоялась",
};
export const personalConsultationLabel: Record<NonNullable<ClientPersonalBooking["consultation_state"]>, string> = {
  SCHEDULED:"Запланирована",READY:"Участники готовы",IN_PROGRESS:"Идёт",
  RECOVERING:"Восстанавливается",TECHNICAL_FAILURE:"Технический сбой",COMPLETED:"Завершена",
};
