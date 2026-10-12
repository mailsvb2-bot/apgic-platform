import { expect, test } from "@playwright/test";

test("quick start leads to canonical specialist availability and a persisted hold", async ({ page, browser }) => {
  test.setTimeout(90_000);
  await page.goto("/");
  await page.getByRole("button", { name: "Проблемы со сном", exact: true }).click();
  await expect(page.getByLabel("С чем нужна помощь")).toHaveValue(/засыпать/);
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await expect(page.getByRole("heading", { name: "Правильно ли мы вас поняли?" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Сон" })).toBeChecked();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();
  await expect(page.getByRole("heading", { level: 3, name: "Марина Лебедева" })).toBeVisible();
  await page.getByRole("button", { name: "Выбрать время у Марина Лебедева" }).click();
  const slots = page.getByRole("button", { name: /Удержать слот/ });
  await expect(slots.first()).toBeVisible();

  let held = false;
  for (let i = 0, count = await slots.count(); i < count; i++) {
    await slots.nth(i).click();
    const success = page.getByRole("heading", { name: "Слот удерживается" });
    const error = page.locator(".alert");
    await expect(success.or(error)).toBeVisible();
    if (await success.isVisible()) {
      held = true;
      break;
    }
  }
  expect(held, "a canonical slot hold must succeed").toBeTruthy();
  await expect(page.getByText(/^Бронь \S+ в состоянии HELD\./)).toBeVisible();
  await expect(page.getByText("APGIC не принимает деньги.")).toBeVisible();
  const activeHoldEndpoint = new URL(`/v1/slot-holds/${await page.evaluate(() => sessionStorage.getItem("apgic:current-hold-id"))}`, page.url()).toString();
  const before = await page.evaluate(async (url) => {
    const response = await fetch(url, { credentials: "include", cache: "no-store" });
    if (!response.ok) throw new Error(`Status fetch ${response.status}`);
    return response.json();
  }, activeHoldEndpoint);
  await page.route(activeHoldEndpoint, async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...before, state: "EXPIRED", booking_state: "EXPIRED" }) });
  });
  await page.getByRole("button", { name: "Проверить статус брони" }).click();
  await expect(page.getByRole("heading", { name: "Актуальное состояние брони" })).toBeVisible();
  await expect(page.getByText(/в состоянии EXPIRED/)).toBeVisible();
  await page.unroute(activeHoldEndpoint);
  await page.route(activeHoldEndpoint, async (route) => {
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ message_safe: "API unavailable" }) });
  });
  await page.getByRole("button", { name: "Проверить статус брони" }).click();
  await expect(page.getByRole("heading", { name: "Статус брони не проверен" })).toBeVisible();
  await expect(page.getByText("Последний известный статус")).toBeVisible();
  await expect(page.getByText("Актуальный статус на сервере не подтверждён.", { exact: false })).toBeVisible();
  await page.unroute(activeHoldEndpoint);
  await page.getByRole("button", { name: "Проверить статус брони" }).click();
  await expect(page.getByRole("heading", { name: "Слот удерживается" })).toBeVisible();
  await expect(page.getByText("Последний известный статус")).toHaveCount(0);

  // A confirmed booking cannot present another checkout instruction.
  await page.route(activeHoldEndpoint, async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      ...before, state: "CONFIRMED", booking_state: "CONFIRMED",
    }) });
  });
  await page.getByRole("button", { name: "Проверить статус брони" }).click();
  await expect(page.getByText("Запись подтверждена. Повторное оформление оплаты для неё недоступно.")).toBeVisible();
  await expect(page.locator(".payment-options button")).toHaveCount(0);
  await page.unroute(activeHoldEndpoint);
  await page.getByRole("button", { name: "Проверить статус брони" }).click();
  await expect(page.getByRole("heading", { name: "Слот удерживается" })).toBeVisible();

  const holdID = await page.evaluate(() => sessionStorage.getItem("apgic:current-hold-id"));
  expect(holdID).toBeTruthy();
  const endpoint = new URL(`/v1/slot-holds/${holdID}`, page.url()).toString();
  const outsider = await browser.newContext();
  try {
    const unauthorized = await outsider.request.get(endpoint);
    expect(unauthorized.status()).toBe(401);
  } finally {
    await outsider.close();
  }

  await page.reload();
  await expect(page.getByRole("heading", { name: "Состояние предыдущей записи" })).toBeVisible();
  await expect(page.getByText(/статус HELD/)).toBeVisible();
  await expect(page.getByText("Это не подтверждённая запись.", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "Выбрать Карта через внешнего провайдера" })).toHaveCount(0);
  const saved = await page.evaluate(async (url) => {
    const current = await fetch(url, { credentials: "include", cache: "no-store" });
    if (!current.ok) throw new Error(`Owner read failed: ${current.status}`);
    return current.json();
  }, endpoint);
  await page.route(endpoint, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ...saved, state: "EXPIRED", booking_state: "EXPIRED" }),
    });
  });
  await page.getByRole("button", { name: "Проверить актуальный статус" }).click();
  await expect(page.getByText(/статус EXPIRED/)).toBeVisible();
  await expect(page.getByText(/статус HELD/)).toHaveCount(0);

  // Resume card must land on the actual restored booking, not restart discovery.
  await page.unroute(endpoint);
  await page.reload();
  const resumeCard = page.getByRole("region", { name: "Продолжить предыдущую запись" });
  await expect(resumeCard).toBeVisible();
  await expect(resumeCard.getByText("Это не подтверждённая запись", { exact: false })).toBeVisible();
  await resumeCard.getByRole("link", { name: "Посмотреть мою запись" }).click();
  await expect(page).toHaveURL(/#previous-booking$/);
  await expect(page.locator("#previous-booking")).toBeInViewport();

  // An outage is not an active booking; one click retries the canonical owner read
  // without reloading or pretending that the external payment was confirmed.
  await page.route(endpoint, async (route) => {
    await route.fulfill({ status: 503, contentType: "application/json", body: "{}" });
  });
  await page.reload();
  await expect(resumeCard.getByRole("button", { name: "Повторить проверку" })).toBeVisible();
  await expect(resumeCard.getByRole("link", { name: "Посмотреть мою запись" })).toHaveCount(0);
  await page.unroute(endpoint);
  await resumeCard.getByRole("button", { name: "Повторить проверку" }).click();
  await expect(resumeCard.getByRole("link", { name: "Посмотреть мою запись" })).toBeVisible();

  // A restored CONFIRMED booking alone must never display consultation facts.
  // Instead open the canonical authenticated booking + order read model.
  // The test exercises the complete state matrix on that single route:
  // unpaid / unavailable / forged / provider-confirmed / refunded or revoked.
  await page.route(endpoint, async (route) => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({ ...saved, state: "CONFIRMED", booking_state: "CONFIRMED" }),
  }));
  await page.getByRole("button", { name: "Проверить актуальный статус" }).click();
  await expect(page.getByRole("region", {name: "Результат консультации"})).toHaveCount(0);
  const paidBookingLink = page.getByRole("link", {name: "Проверить запись и оплату"});
  await expect(paidBookingLink).toHaveAttribute("href", `/bookings/${saved.booking_id}`);

  const bookingReadEndpoint = new URL(`/v1/bookings/${saved.booking_id}`, page.url()).toString();
  const orderReadEndpoint = new URL(`/v1/bookings/${saved.booking_id}/order`, page.url()).toString();
  const resultEndpoint = new URL(`/v1/consultations/${saved.booking_id}/result`, page.url()).toString();
  await page.route(bookingReadEndpoint, route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      booking_id: saved.booking_id, hold_id: saved.id,
      state: "CONFIRMED", hold_expires_at: saved.expires_at,
      starts_at: "2030-11-12T11:00:00Z", ends_at: "2030-11-12T12:00:00Z",
    }),
  }));
  let paymentMode: "pending" | "captured" | "unavailable" | "missing" = "pending";
  await page.route(orderReadEndpoint, async route => {
    if (paymentMode === "unavailable") return route.fulfill({status: 503, body: "{}"});
    if (paymentMode === "missing") return route.fulfill({status: 404, body: "{}"});
    return route.fulfill({
      status: 200, contentType: "application/json",
      body: JSON.stringify({
        order_id: "ord-for-booking-" + saved.booking_id, booking_id: saved.booking_id,
        booking_state: "CONFIRMED",
        payment_state: paymentMode === "captured" ? "CAPTURED" : "PENDING_PROVIDER",
        amount_minor: 150000, currency: "RUB",
        provider_id: "external-provider", payment_method: "SBP",
      }),
    });
  });
  await paidBookingLink.click();
  await expect(page.getByRole("heading", {name: "Моя запись"})).toBeVisible();
  const closed = page.getByRole("alert").filter({hasText: "Доступ к консультации закрыт"});
  await expect(closed).toBeVisible();
  await expect(page.getByRole("region", {name: "Результат консультации"})).toHaveCount(0);
  for (const unsafe of ["unavailable", "missing"] as const) {
    paymentMode = unsafe;
    await page.getByRole("button", {name: "Проверить актуальный статус"}).click();
    await expect(closed).toBeVisible();
    await expect(page.getByRole("region", {name: "Результат консультации"})).toHaveCount(0);
  }

  paymentMode = "captured";
  await page.getByRole("button", {name: "Проверить актуальный статус"}).click();
  await expect(page.getByRole("region", {name: "Подтверждённые сведения о платеже"})).toContainText("Списание подтверждено");
  const consultation = page.getByRole("region", {name: "Результат консультации"});
  await expect(consultation).toBeVisible();
  await expect(consultation.getByText(/Состояние консультации: Завершена/)).toHaveCount(0);

  let mode: "outage" | "foreign" | "denied" | "completed" = "outage";
  const trustedResult = {
    booking_id: saved.booking_id, state: "COMPLETED",
    provider_instance_id: "00000000-0000-4000-8000-000000000001",
    completion_evidence_ref: "provider-evidence/ENDED/SYSTEM",
  };
  await page.route(resultEndpoint, async route => {
    if (mode === "outage") return route.fulfill({status: 503, body: "{}"});
    if (mode === "denied") return route.fulfill({status: 403, body: "{}"});
    const data = mode === "foreign" ? {...trustedResult, booking_id: "someone-else"} : trustedResult;
    return route.fulfill({status: 200, contentType: "application/json", body: JSON.stringify(data)});
  });

  await consultation.getByRole("button", {name: "Проверить результат консультации"}).click();
  await expect(consultation.getByRole("alert")).toContainText("Не удалось подтвердить результат");
  mode = "foreign";
  await consultation.getByRole("button", {name: "Проверить результат консультации"}).click();
  await expect(consultation.getByRole("alert")).toContainText("Не удалось подтвердить результат");
  mode = "denied";
  await consultation.getByRole("button", {name: "Проверить результат консультации"}).click();
  await expect(consultation.getByRole("alert")).toContainText("Нет подтверждённого доступа");
  mode = "completed";
  await consultation.getByRole("button", {name: "Проверить результат консультации"}).click();
  await expect(consultation.getByText("Состояние консультации: Завершена.")).toBeVisible();
  await expect(consultation.getByText("Подтверждение провайдера: provider-evidence/ENDED/SYSTEM.")).toBeVisible();

  await page.reload();
  await expect(consultation).toBeVisible();
  await expect(consultation.getByText("Состояние консультации: Завершена.")).toHaveCount(0);
  await consultation.getByRole("button", {name: "Проверить результат консультации"}).click();
  await expect(consultation.getByText("Состояние консультации: Завершена.")).toBeVisible();

  // A payment read that loses capture revokes all result actions on refresh.
  paymentMode = "pending";
  await page.getByRole("button", {name: "Проверить актуальный статус"}).click();
  await expect(closed).toBeVisible();
  await expect(consultation).toHaveCount(0);
  await page.unroute(endpoint);
  await page.goto("/");
  await expect(consultation).toHaveCount(0);

});
