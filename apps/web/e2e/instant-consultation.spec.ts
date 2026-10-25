import { expect, test } from "@playwright/test";

test("Duty/Now requires a confirmed request and cannot create unpaid consultation access", async ({ page }) => {
  await page.goto("/now");
  await expect(page.getByRole("heading", { name: "Поговорить со специалистом сейчас" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Найти дежурного" })).toHaveCount(0);

  await page.getByLabel("С чем хотите обратиться?").fill("Мне тревожно, хочу поговорить о сне");
  await page.getByRole("button", { name: "Продолжить" }).click();
  await expect(page.getByRole("heading", { name: "Проверьте направление" })).toBeVisible();
  await page.getByLabel("Тема запроса").selectOption("sleep");
  await page.getByRole("button", { name: "Подтвердить тему" }).click();
  await expect(page.getByRole("heading", { name: "Запрос подтверждён" })).toBeVisible();

  await page.getByRole("button", { name: "Найти дежурного" }).click();
  // The isolated CI backend has no real payments or qualified duty supply.
  // A client cannot be offered a fake consultation.
  await expect(page.locator('section[aria-label="Быстрая консультация"]').getByRole("alert")).toContainText("Консультации сейчас пока недоступны");
  await expect(page.getByText("Специалист принял запрос")).toHaveCount(0);
});

test("Specialist Duty/Now is visible but does not claim unverified availability", async ({ page }) => {
  await page.goto("/specialist/now");
  await expect(page.getByRole("heading", { name: "Принимать сейчас" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Принимать сейчас/ })).toBeDisabled();
});
