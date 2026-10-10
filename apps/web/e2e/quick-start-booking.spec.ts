import { expect, test } from "@playwright/test";

test("quick start leads to canonical specialist availability and a persisted hold", async ({ page, browser }) => {
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
  await expect(page.getByRole("heading", { name: "Актуальное состояние брони" })).toBeVisible();
  await expect(page.getByText("Последний известный статус")).toHaveCount(0);

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


});
