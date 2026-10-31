import type { FinanceOrderStatus } from "../../../packages/contracts/src/generated/apgic-v1";
import type { BookingClientConfig } from "./mobile-booking-client.ts";

type FinanceFetch = (url: string, init?: {
  credentials?: "include";
  headers?: Record<string, string>;
  cache?: "no-store";
}) => Promise<{ status: number; json(): Promise<unknown> }>;

// Owner-facing, provider-executed financial status. APGIC never holds funds:
// every label describes a provider fact or an APGIC instruction, never a
// balance, wallet or payable amount held by APGIC.
export const settlementLabels: Record<FinanceOrderStatus["settlement_status"], string> = {
  NOT_INSTRUCTED: "Расчёт ещё не начат",
  HELD: "Расчёт приостановлен до проверки",
  INSTRUCTED: "Инструкция расчёта передана провайдеру",
  PROVIDER_SETTLED: "Провайдер подтвердил расчёт",
};
export const payoutLabels: Record<FinanceOrderStatus["payout_status"], string> = {
  NOT_ELIGIBLE: "Выплата пока недоступна",
  AWAITING_PROVIDER_ELIGIBILITY: "Ожидаем проверку провайдера",
  BLOCKED: "Провайдер не подтвердил выплату",
  INSTRUCTED: "Инструкция выплаты передана провайдеру",
  PROVIDER_PAID_OUT: "Провайдер подтвердил выплату",
};
export const reconciliationLabels: Record<FinanceOrderStatus["reconciliation_outcome"], string> = {
  NOT_RECONCILED: "Сверка ещё не проводилась",
  MATCHED: "Сверка с провайдером сошлась",
  MISMATCH: "Обнаружено расхождение; им занимается финансовая служба",
};
export const discrepancyLabels: Record<FinanceOrderStatus["discrepancy_status"], string> = {
  NONE: "Расхождений нет",
  OPEN: "Расхождение на разборе у финансовой службы",
  RESOLVED: "Расхождение закрыто с подтверждением",
};

const optionalMinor = (value: unknown) =>
  value === undefined || (typeof value === "number" && Number.isSafeInteger(value) && value >= 0);

export function requireOwnedFinanceStatus(payload: unknown, orderID: string): FinanceOrderStatus {
  if (!payload || typeof payload !== "object") throw new Error("FINANCE_STATUS_INVALID");
  const v = payload as Partial<FinanceOrderStatus>;
  if (v.order_id !== orderID || v.execution_owner !== "EXTERNAL_PROVIDER" ||
      typeof v.currency !== "string" || !/^[A-Z]{3}$/.test(v.currency) ||
      typeof v.total_amount_minor !== "number" || !Number.isSafeInteger(v.total_amount_minor) || v.total_amount_minor < 0 ||
      !optionalMinor(v.platform_fee_minor) || !optionalMinor(v.beneficiary_amount_minor) ||
      typeof v.disputed_amount_minor !== "number" || !Number.isSafeInteger(v.disputed_amount_minor) || v.disputed_amount_minor < 0 ||
      !(typeof v.settlement_status === "string" && v.settlement_status in settlementLabels) ||
      !(typeof v.payout_status === "string" && v.payout_status in payoutLabels) ||
      !(typeof v.reconciliation_outcome === "string" && v.reconciliation_outcome in reconciliationLabels) ||
      !(typeof v.discrepancy_status === "string" && v.discrepancy_status in discrepancyLabels) ||
      (v.platform_fee_minor !== undefined && v.beneficiary_amount_minor !== undefined &&
        v.platform_fee_minor + v.beneficiary_amount_minor !== v.total_amount_minor) ||
      (v.payout_status === "PROVIDER_PAID_OUT" && v.settlement_status !== "PROVIDER_SETTLED")) {
    throw new Error("FINANCE_STATUS_INVALID");
  }
  for (const key of Object.keys(v)) {
    if (/balance|wallet|custody|escrow/i.test(key)) throw new Error("FINANCE_STATUS_INVALID");
  }
  return v as FinanceOrderStatus;
}

export async function readOwnedFinanceStatus(
  config: BookingClientConfig,
  orderID: string,
  request: FinanceFetch = (url, init) => fetch(url, init),
): Promise<FinanceOrderStatus> {
  if (!orderID.trim() || /[/?#]/.test(orderID)) throw new Error("FINANCE_ORDER_ID_INVALID");
  let origin: URL;
  try { origin = new URL(config.baseURL); } catch { throw new Error("FINANCE_ORIGIN_INVALID"); }
  const local = origin.protocol === "http:" && (origin.hostname === "localhost" || origin.hostname === "127.0.0.1");
  if ((origin.protocol !== "https:" && !local) || origin.username || origin.password ||
      origin.search || origin.hash || (origin.pathname && origin.pathname !== "/")) {
    throw new Error("FINANCE_ORIGIN_INVALID");
  }
  const response = await request(origin.origin + "/v1/finance/orders/" + encodeURIComponent(orderID) + "/status", {
    credentials: "include",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(config.sessionCookie ? { Cookie: config.sessionCookie } : {}),
    },
  });
  if (response.status === 401 || response.status === 403) throw new Error("FINANCE_SESSION_REQUIRED");
  if (response.status === 404) throw new Error("FINANCE_ORDER_NOT_FOUND");
  if (response.status !== 200) throw new Error("FINANCE_STATUS_UNAVAILABLE");
  return requireOwnedFinanceStatus(await response.json(), orderID);
}

export function formatMinor(minor: number, currency: string): string {
  const whole = Math.trunc(minor / 100);
  const fraction = String(Math.abs(minor % 100)).padStart(2, "0");
  return `${whole.toLocaleString("ru-RU")},${fraction} ${currency}`;
}
