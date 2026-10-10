import { expect, test } from "@playwright/test";

test("expired displayed hold warns without claiming server cancellation", async ({ page }) => {
  const fixtureHoldID = "00000000-0000-4000-8000-00000000f188";
  await page.route("**/v1/meta", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ conformance_provider_events: false }),
    });
  });
  await page.route("**/v1/slot-holds", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        id: fixtureHoldID,
        booking_id: "00000000-0000-4000-8000-00000000b188",
        state: "ACTIVE",
        booking_state: "HELD",
        expires_at: "2020-01-01T00:00:00Z",
        reason_code: "HELD",
      }),
    });
  });
  await page.route(`**/v1/slot-holds/${fixtureHoldID}/checkout-options`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ options: [] }),
    });
  });
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Бессонница и проблемы со сном");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await expect(page.getByRole("checkbox", { name: "Сон" })).toBeChecked();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();
  await page.getByRole("button", { name: "Выбрать время у Марина Лебедева" }).click();
  const slot = page.getByRole("button", { name: /Удержать слот/ }).first();
  await expect(slot).toBeVisible();
  await slot.click();
  await expect(page.getByRole("heading", { name: "Слот удерживается" })).toBeVisible();
  await expect(page.getByText("Онлайн-оплата через внешнего исполнителя пока недоступна.", { exact: false })).toBeVisible();
  await expect(page.getByText("Время временно удерживается, но запись ещё не подтверждена.", { exact: false })).toBeVisible();
  await expect(page.getByText("Завершите оплату до окончания удержания.")).toHaveCount(0);
  await expect(page.getByText("Указанное время удержания прошло.", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: /Выбрать Карта через внешнего провайдера/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Выбрать СБП через внешнего провайдера/ })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Поручение на оплату создано" })).toHaveCount(0);
});
