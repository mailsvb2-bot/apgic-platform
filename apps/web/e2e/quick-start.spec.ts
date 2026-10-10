import { expect, test } from "@playwright/test";

const scenarios = [
  { name: "Тревога и стресс", phrase: "тревожусь" },
  { name: "Проблемы со сном", phrase: "засыпать" },
  { name: "Отношения", phrase: "отношения" },
  { name: "Работа и карьера", phrase: "карьерных" },
];

for (const scenario of scenarios) {
  test(`quick start "${scenario.name}" is editable and only submits after consentful action`, async ({ page }) => {
    await page.goto("/");
    let created = 0;
    page.on("request", (request) => {
      if (request.method() === "POST" && new URL(request.url()).pathname === "/v1/help-intents") created++;
    });

    const button = page.getByRole("button", { name: scenario.name, exact: true });
    const request = page.getByLabel("С чем нужна помощь");
    await expect(button).toBeVisible();
    await button.click();

    await expect(button).toHaveAttribute("aria-pressed", "true");
    await expect(request).toHaveValue(new RegExp(scenario.phrase));
    expect(created).toBe(0);

    await request.fill("Мне тревожно перед выступлениями");
    await expect(button).toHaveAttribute("aria-pressed", "false");
    expect(created).toBe(0);

    await page.getByRole("button", { name: "Разобрать запрос" }).click();
    await expect(page.getByRole("heading", { name: "Правильно ли мы вас поняли?" })).toBeVisible();
    expect(created).toBe(1);
    await expect(page.getByRole("button", { name: "Подтвердить и показать специалистов" })).toBeVisible();
  });
}
