import { expect, test } from "@playwright/test";

for (const topic of ["Тревога и стресс", "Проблемы со сном", "Отношения", "Работа и карьера"]) {
  test(`default quick-start "${topic}" is accepted by the canonical intent API`, async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: topic, exact: true }).click();
    await page.getByRole("button", { name: "Разобрать запрос" }).click();
    await expect(page.getByRole("heading", { name: "Правильно ли мы вас поняли?" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Подтвердить и показать специалистов" })).toBeVisible();
  });
}
