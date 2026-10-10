import { expect, test } from "@playwright/test";

test("quick start leads to canonical specialist availability and a persisted hold", async ({ page }) => {
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
  await expect(page.getByText(/^Бронь \\S+ в состоянии HELD\\./)).toBeVisible();
  await expect(page.getByText("APGIC не принимает деньги.")).toBeVisible();
});
