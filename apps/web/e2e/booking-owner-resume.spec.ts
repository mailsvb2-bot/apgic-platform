import { expect, test } from "@playwright/test";

// The browser contract is simulated; the authenticated PostgreSQL owner read
// and API restart semantics are covered by Go domain/HTTP integration tests.
test("direct booking page recovers from canonical owner API without sessionStorage token", async ({ page }) => {
  const id = "book-direct-resume";
  let mode: "confirmed" | "outage" | "foreign" | "denied" | "cancelled" = "confirmed";
  let paymentMode: "captured" | "pending" | "outage" | "missing" = "captured";
  await page.route(`**/v1/bookings/${id}`, async (route) => {
    if (mode === "outage") return route.fulfill({ status: 503, body: "{}" });
    if (mode === "denied") return route.fulfill({ status: 401, body: "{}" });
    const payload = {
      booking_id: mode === "foreign" ? "another-client-booking" : id,
      state: mode === "cancelled" ? "CANCELLED" : "CONFIRMED",
      hold_id: "hold-direct-resume",
      hold_expires_at: "2026-10-12T18:00:00Z",
      starts_at: "2026-10-15T11:00:00Z",
      ends_at: "2026-10-15T12:00:00Z",
    };
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(payload) });
  });

  await page.route(`**/v1/bookings/${id}/order`, async (route) => {
    if (paymentMode === "outage") return route.fulfill({status: 503, body: "{}"});
    if (paymentMode === "missing") return route.fulfill({status: 404, body: "{}"});
    const economic = {
      order_id: "ord-direct-resume", booking_id: id,
      booking_state: mode === "cancelled" ? "CANCELLED" : "CONFIRMED",
      payment_state: paymentMode === "pending" ? "PENDING_PROVIDER" : "CAPTURED",
      amount_minor: 150000, currency: "RUB",
      provider_id: "provider-external", payment_method: "SBP",
      ...(mode === "cancelled" ? {refund_state: "SUCCEEDED"} : {}),
    };
    return route.fulfill({status: 200, contentType: "application/json", body: JSON.stringify(economic)});
  });

  await page.goto(`/bookings/${id}`);
  expect(await page.evaluate(() => sessionStorage.length)).toBe(0);
  await expect(page.getByRole("heading", { name: "Моя запись" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Запись подтверждена" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Подтверждённые сведения о платеже" })).toContainText("Списание подтверждено");
  await expect(page.getByRole("region", { name: "Результат консультации" })).toBeVisible();

  // A booking can carry CONFIRMED while the payment read is pending,
  // missing, or unavailable. None of these states may expose consultation.
  for (const uncertain of ["pending", "outage", "missing"] as const) {
    paymentMode = uncertain;
    await page.getByRole("button", {name: "Проверить актуальный статус"}).click();
    await expect(page.getByRole("heading", {name: "Запись подтверждена"})).toBeVisible();
    // Unknown payment must deny a NEW media grant, but preserve the separate provider history read.
    await expect(page.getByRole("region", {name: "Результат консультации"})).toBeVisible();
    await expect(page.getByRole("status").filter({hasText: "Вход в защищённую медиакомнату недоступен"})).toBeVisible();
    await expect(page.getByText("Состояние консультации: Завершена.")).toHaveCount(0);
  }
  paymentMode = "captured";
  await page.getByRole("button", {name: "Проверить актуальный статус"}).click();
  await expect(page.getByRole("region", {name: "Результат консультации"})).toBeVisible();

  // An upstream outage cannot silently replay the previously confirmed state.
  mode = "outage";
  await page.getByRole("button", { name: "Проверить актуальный статус" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Состояние не подтверждено" })).toContainText("Состояние не подтверждено");
  await expect(page.getByRole("heading", { name: "Запись подтверждена" })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Результат консультации" })).toHaveCount(0);

  mode = "foreign";
  await page.getByRole("button", { name: "Проверить актуальный статус" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Состояние не подтверждено" })).toContainText("Состояние не подтверждено");
  await expect(page.getByRole("heading", { name: "Запись подтверждена" })).toHaveCount(0);

  mode = "cancelled";
  await page.getByRole("button", { name: "Проверить актуальный статус" }).click();
  await expect(page.getByRole("heading", { name: "Запись отменена" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Подтверждённые сведения о платеже" })).toContainText("SUCCEEDED");
  await expect(page.getByRole("region", { name: "Результат консультации" })).toHaveCount(0);

  mode = "denied";
  await page.reload();
  await expect(page.getByRole("alert").filter({ hasText: "необходимо войти" })).toContainText("необходимо войти");
  await expect(page.getByRole("heading", { name: "Запись отменена" })).toHaveCount(0);
});
