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
  const firstLabel = await firstSlot.textContent();
  await firstSlot.click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();

  // Switching removes all actionable state tied to the previous specialist.
  await ilya.click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Выберите время с Илья Соколов"})).toBeVisible();
  await expect(page.getByRole("button", {name: /Удержать слот/}).first()).toBeVisible();
  if (firstLabel) {
    await expect(page.getByRole("button", {name: firstLabel.trim(), exact: true})).toHaveCount(0);
  }

  // The original hold still occupies its slot on the server. Returning to
  // the first specialist must not advertise the same slot as newly free.
  await marina.click();
  await expect(page.getByRole("heading", {name: "Выберите время с Марина Лебедева"})).toBeVisible();
  await expect(page.getByRole("button", {name: /Удержать слот/}).first()).toBeVisible();
  if (firstLabel) {
    await expect(page.getByRole("button", {name: firstLabel.trim(), exact: true})).toHaveCount(0);
  }
  await expect(page.getByRole("heading", {name: "Поручение на оплату создано"})).toHaveCount(0);
  await expect(page.getByRole("heading", {name: "Бронь подтверждена провайдером"})).toHaveCount(0);
});
