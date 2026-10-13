import { expect, test } from "@playwright/test";

async function confirmedSpecialists(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Мне тревожно перед выступлениями");
  await page.getByRole("button", {name: "Разобрать запрос"}).click();
  await expect(page.getByRole("checkbox", {name: "Тревога и волнение"})).toBeChecked();
  await page.getByRole("button", {name: "Подтвердить и показать специалистов"}).click();
  await expect(page.getByRole("button", {name: "Выбрать время у Марина Лебедева"})).toBeVisible();
  await expect(page.getByRole("button", {name: "Выбрать время у Илья Соколов"})).toBeVisible();
}

test("delayed hold-status read never resurrects a checkout after changing specialist", async ({page}) => {
  await confirmedSpecialists(page);
  await page.getByRole("button", {name: "Выбрать время у Марина Лебедева"}).click();
  await page.getByRole("button", {name: /Удержать слот/}).first().click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();

  let started: (() => void) | undefined;
  let release: (() => void) | undefined;
  const begun = new Promise<void>(resolve => { started = resolve; });
  const released = new Promise<void>(resolve => { release = resolve; });
  await page.route("**/v1/slot-holds/*", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    started?.();
    await released;
    await route.fulfill({
      status: 200, contentType: "application/json",
      body: JSON.stringify({
        id: route.request().url().split("/").pop(),
        booking_id: "old-booking", state: "CONSUMED",
        booking_state: "CONFIRMED",
        expires_at: "2030-10-12T18:00:00Z",
        reason_code: "OLD_CONFIRMED",
      }),
    });
  });
  await page.getByRole("button", {name: "Проверить статус брони"}).click();
  await begun;
  await page.getByRole("button", {name: "Выбрать время у Илья Соколов"}).click();
  await expect(page.getByRole("heading", {name: "Выберите время с Илья Соколов"})).toBeVisible();
  const finished = page.waitForResponse(res =>
    res.url().includes("/v1/slot-holds/") && res.request().method() === "GET");
  release?.();
  await finished;
  await expect(page.getByRole("heading", {name: "Актуальное состояние брони"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Бронь подтверждена провайдером"})).toHaveCount(0);
});

test("delayed payment-status read never revives previous order after changing specialist", async ({page}) => {
  await confirmedSpecialists(page);
  await page.getByRole("button", {name: "Выбрать время у Марина Лебедева"}).click();
  await page.getByRole("button", {name: /Удержать слот/}).first().click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();
  await page.getByRole("button", {name: /Выбрать СБП через внешнего провайдера/}).click();
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toBeVisible();

  let started: (() => void) | undefined;
  let release: (() => void) | undefined;
  const begun = new Promise<void>(resolve => { started = resolve; });
  const released = new Promise<void>(resolve => { release = resolve; });
  await page.route("**/v1/orders/*", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    started?.();
    await released;
    await route.fulfill({
      status: 200, contentType: "application/json",
      body: JSON.stringify({
        order_id: route.request().url().split("/").pop(),
        booking_id: "old-booking",
        booking_state: "CONFIRMED",
        payment_state: "CAPTURED",
        amount_minor: 1, currency: "RUB",
        provider_id: "old-provider", payment_method: "SBP",
      }),
    });
  });
  await page.getByRole("button", {name: "Обновить состояние оплаты"}).click();
  await begun;
  await page.getByRole("button", {name: "Выбрать время у Илья Соколов"}).click();
  await expect(page.getByRole("heading", {name: "Выберите время с Илья Соколов"})).toBeVisible();
  const finished = page.waitForResponse(res =>
    res.url().includes("/v1/orders/") && res.request().method() === "GET");
  release?.();
  await finished;
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Бронь подтверждена провайдером"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toHaveCount(0);
  await expect(page.getByRole("button", {name: "Зафиксировать подтверждение внешнего провайдера (только тест)"})).toHaveCount(0);
});
