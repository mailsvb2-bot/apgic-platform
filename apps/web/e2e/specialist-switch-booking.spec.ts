import { expect, test } from "@playwright/test";

test("switching specialist cannot keep previous slot hold, checkout or payment actions", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Мне тревожно перед выступлениями");
  await page.getByRole("button", {name: "Разобрать запрос"}).click();
  await expect(page.getByRole("checkbox", {name: "Тревога и волнение"})).toBeChecked();
  await page.getByRole("button", {name: "Подтвердить и показать специалистов"}).click();

  const marina = page.getByRole("button", {name: "Выбрать время у Марина Лебедева"});
  const ilya = page.getByRole("button", {name: "Выбрать время у Илья Соколов"});
  await expect(marina).toBeVisible();
  await expect(ilya).toBeVisible();

  await marina.click();
  const firstSlot = page.getByRole("button", {name: /Удержать слот/}).first();
  await expect(firstSlot).toBeVisible();
  const firstSlotID = await firstSlot.getAttribute("data-slot-id");
  expect(firstSlotID, "slot must have stable server identifier").toBeTruthy();
  await firstSlot.click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();

  // Switching removes all actionable state tied to the previous specialist.
  await ilya.click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Выберите время с Илья Соколов"})).toBeVisible();
  await expect(page.getByRole("button", {name: /Удержать слот/}).first()).toBeVisible();
  await expect(page.locator(`button[data-slot-id="${firstSlotID}"]`)).toHaveCount(0);

  // The original hold still occupies its slot on the server. Returning to
  // the first specialist must not advertise the same slot as newly free.
  await marina.click();
  await expect(page.getByRole("heading", {name: "Выберите время с Марина Лебедева"})).toBeVisible();
  await expect(page.getByRole("button", {name: /Удержать слот/}).first()).toBeVisible();
  await expect(page.locator(`button[data-slot-id="${firstSlotID}"]`)).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Бронь подтверждена провайдером"})).toHaveCount(0);
});

test("out-of-order specialist schedule responses cannot switch the selected expert", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Мне тревожно перед выступлениями");
  await page.getByRole("button", {name: "Разобрать запрос"}).click();
  await expect(page.getByRole("checkbox", {name: "Тревога и волнение"})).toBeChecked();
  await page.getByRole("button", {name: "Подтвердить и показать специалистов"}).click();
  const marina = page.getByRole("button", {name: "Выбрать время у Марина Лебедева"});
  const ilya = page.getByRole("button", {name: "Выбрать время у Илья Соколов"});
  await expect(marina).toBeVisible();
  await expect(ilya).toBeVisible();

  let requested: (() => void) | undefined;
  let resolveDelayed: (() => void) | undefined;
  const requestArrived = new Promise<void>(resolve => { requested = resolve; });
  const releaseDelayed = new Promise<void>(resolve => { resolveDelayed = resolve; });
  await page.route("**/v1/specialists/spec-lebedeva/slots**", async route => {
    requested?.();
    await releaseDelayed;
    await route.fulfill({
      status: 200, contentType: "application/json",
      body: JSON.stringify({
        specialist_id: "spec-lebedeva",
        slots: [{
          id: "stale-marina-id",
          starts_at: "2030-10-12T12:00:00Z",
          ends_at: "2030-10-12T13:00:00Z",
          exclusive: true,
        }],
      }),
    });
  });
  await marina.click();
  await requestArrived;
  await ilya.click();
  await expect(page.getByRole("heading", {name: "Выберите время с Илья Соколов"})).toBeVisible();
  await expect(page.getByRole("button", {name: /Удержать слот/}).first()).toBeVisible();
  resolveDelayed?.();
  await expect(page.getByRole("heading", {name: "Выберите время с Марина Лебедева"})).toHaveCount(0);
  await expect(page.locator('button[data-slot-id="stale-marina-id"]')).toHaveCount(0);
});
