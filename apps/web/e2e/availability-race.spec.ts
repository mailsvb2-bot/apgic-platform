import { expect, test } from "@playwright/test";

test("a late specialist response cannot resurrect old checkout after replacing help intent", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Нужна помощь со сном");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();
  await expect(page.getByRole("button", { name: "Выбрать время у Марина Лебедева" })).toBeVisible();

  let release: (() => void) | undefined;
  let started: (() => void) | undefined;
  const requestStarted = new Promise<void>((resolve) => { started = resolve; });
  const responseAllowed = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/v1/specialists/spec-lebedeva/slots**", async (route) => {
    started?.();
    await responseAllowed;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        specialist_id: "spec-lebedeva",
        slots: [{
          id: "old-slot", starts_at: "2030-10-12T12:00:00Z",
          ends_at: "2030-10-12T13:00:00Z", exclusive: true,
        }],
      }),
    });
  });
  await page.getByRole("button", { name: "Выбрать время у Марина Лебедева" }).click();
  await requestStarted;
  await page.getByLabel("С чем нужна помощь").fill("Хочу обсудить карьеру");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  release?.();
  await expect(page.getByRole("heading", {name: "Выберите время с Марина Лебедева"})).toHaveCount(0);
  await expect(page.getByRole("button", {name: /Удержать слот/})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
});
