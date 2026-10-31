import assert from "node:assert/strict";
import test from "node:test";
import { formatMinor, readOwnedFinanceStatus, requireOwnedFinanceStatus } from "./mobile-finance-status-client.ts";

const orderID = "00000000-0000-4000-8000-000000000450";
const status = {
  order_id: orderID, currency: "RUB", total_amount_minor: 20000,
  commission_policy_version: "commission-r2-ci-v1", platform_fee_minor: 2000, beneficiary_amount_minor: 18000,
  settlement_status: "PROVIDER_SETTLED", payout_status: "PROVIDER_PAID_OUT",
  reconciliation_outcome: "MISMATCH", discrepancy_status: "RESOLVED",
  discrepancy_resolution_kind: "PROVIDER_CORRECTION_CONFIRMED",
  disputed_amount_minor: 0, revenue_assurance: "PASS_WITH_EXCEPTION", revenue_reason_code: "R",
  execution_owner: "EXTERNAL_PROVIDER",
};
const reply = (payload: unknown, code = 200) => async () => ({ status: code, async json() { return payload; } });

test("native finance status reads the owner-scoped provider-executed projection", async () => {
  let url = "", cookie = "", cache = "";
  const actual = await readOwnedFinanceStatus(
    { baseURL: "https://apgic.ru", sessionCookie: "__Host-apgic_session=trusted" }, orderID,
    async (u, init) => { url = u; cookie = init?.headers?.Cookie ?? ""; cache = init?.cache ?? ""; return { status: 200, async json() { return status; } }; },
  );
  assert.equal(actual.discrepancy_status, "RESOLVED");
  assert.equal(url, `https://apgic.ru/v1/finance/orders/${orderID}/status`);
  assert.equal(cookie, "__Host-apgic_session=trusted");
  assert.equal(cache, "no-store");
  assert.equal(formatMinor(18000, "RUB"), "180,00 RUB");
});

test("native finance status fails closed on foreign, custodial or inconsistent payloads", () => {
  for (const bad of [
    { ...status, order_id: "foreign" },
    { ...status, execution_owner: "APGIC" },
    { ...status, discrepancy_status: "IGNORED" },
    { ...status, platform_fee_minor: 3000 },
    { ...status, settlement_status: "INSTRUCTED" },
    { ...status, apgic_balance_minor: 18000 },
    { ...status, total_amount_minor: 1.5 },
  ]) {
    assert.throws(() => requireOwnedFinanceStatus(bad, orderID), /FINANCE_STATUS_INVALID/);
  }
});

test("native finance status maps session, ownership and outage to typed errors", async () => {
  for (const [code, expected] of [
    [401, /FINANCE_SESSION_REQUIRED/], [403, /FINANCE_SESSION_REQUIRED/],
    [404, /FINANCE_ORDER_NOT_FOUND/], [503, /FINANCE_STATUS_UNAVAILABLE/],
  ] as const) {
    await assert.rejects(readOwnedFinanceStatus({ baseURL: "https://apgic.ru" }, orderID, reply({}, code)), expected);
  }
  await assert.rejects(readOwnedFinanceStatus({ baseURL: "http://evil.example" }, orderID, reply(status)), /FINANCE_ORIGIN_INVALID/);
  await assert.rejects(readOwnedFinanceStatus({ baseURL: "https://apgic.ru" }, "../x", reply(status)), /FINANCE_ORDER_ID_INVALID/);
});
