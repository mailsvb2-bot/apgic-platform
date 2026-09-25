import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("help intent journey stays usable and accessible", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  const request = page.getByLabel("С чем нужна помощь");
  await request.fill("Мне тревожно перед выступлениями");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();

  await expect(page.getByText("Это предположение по вашим словам, не диагноз")).toBeVisible();
  await expect(page.getByText("Диагноз не поставлен: нет.")).toBeVisible();

  await page.getByRole("checkbox", { name: "Тревога и волнение" }).uncheck();
  await page.getByRole("checkbox", { name: "Сон" }).check();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();

  const cards = page.getByRole("heading", { level: 3 });
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toHaveText("Марина Лебедева");
  await expect(page.getByText("Илья Соколов")).toHaveCount(0);
  await expect(page.getByText("В проекции: Марина Лебедева.")).toBeVisible();
  await page.getByRole("button", { name: "Сбросить поисковый индекс" }).click();
  await expect(page.getByText("В проекции никого нет.")).toBeVisible();
  await expect(page.getByRole("heading", { level: 3, name: "Марина Лебедева" })).toBeVisible();
  await page.getByRole("button", { name: "Восстановить поиск из каталога" }).click();
  await expect(page.getByText("В проекции: Марина Лебедева.")).toBeVisible();

  await page.getByRole("button", { name: "Выбрать время у Марина Лебедева" }).click();
  const holds = page.getByRole("button", { name: /Удержать слот/ });
  await expect(holds.first()).toBeVisible();
  const count = await holds.count();
  let held = false;
  for (let index = 0; index < count; index += 1) {
    await holds.nth(index).click();
    const success = page.getByRole("heading", { name: "Слот удерживается" });
    const alert = page.locator(".alert");
    await expect(success.or(alert)).toBeVisible();
    if (await success.isVisible()) {
      held = true;
      break;
    }
  }
  expect(held).toBeTruthy();
  await expect(page.getByText(/Бронь book-/)).toBeVisible();
  await expect(page.getByText("APGIC не принимает деньги.")).toBeVisible();
  await page.getByRole("button", { name: "Выбрать Карта через внешнего провайдера" }).click();
  await expect(page.getByRole("heading", { name: "Поручение на оплату создано" })).toBeVisible();
  await expect(page.getByText("APGIC принимает деньги: нет.")).toBeVisible();
  await expect(page.getByText("PENDING_PAYMENT")).toBeVisible();
  await page.getByRole("button", { name: "Зафиксировать подтверждение внешнего провайдера" }).click();
  await expect(page.getByRole("heading", { name: "Бронь подтверждена провайдером" })).toBeVisible();
  await expect(page.getByText("CONFIRMED")).toBeVisible();
  await page.getByRole("button", { name: "Зафиксировать подтверждение внешнего провайдера" }).click();
  await expect(page.getByText("Повтор: уже учтён.")).toBeVisible();
  await expect(page.getByText("Служебное уведомление о брони отправлено без маркетингового согласия.")).toBeVisible();
  await expect(page.getByText("Вход откроется за 15 минут до начала.")).toBeVisible();
  await page.getByRole("button", { name: "Завершить без доказательства" }).click();
  await expect(page.locator(".alert")).toContainText("доказательство провайдера");
  await page.getByRole("button", { name: "Зафиксировать факты входа" }).click();
  await expect(page.getByText("Сессия IN_PROGRESS.")).toBeVisible();
  await page.getByRole("button", { name: "Сообщить о сбое связи" }).click();
  await expect(page.getByText("Сессия RECOVERING.")).toBeVisible();
  await expect(page.getByText("Повторное списание: нет.")).toBeVisible();
  await expect(page.getByText("Сессия COMPLETED.")).toHaveCount(0);
  await page.getByRole("button", { name: "Восстановление удалось" }).click();
  await expect(page.getByText("Сессия IN_PROGRESS.")).toBeVisible();
  await page.getByRole("button", { name: "Завершить по доказательству провайдера" }).click();
  await expect(page.getByText("Сессия COMPLETED.")).toBeVisible();
  await expect(page.getByText("Повторное списание: нет.")).toBeVisible();
  await page.getByRole("button", { name: "Отменить бронь через внешнего провайдера" }).click();
  await expect(page.getByRole("heading", { name: "Бронь отменена" })).toBeVisible();
  await expect(page.getByText("CANCELLED")).toBeVisible();
  await expect(page.getByText("APGIC возвращает деньги: нет.")).toBeVisible();
  await page.getByRole("button", { name: "Отменить бронь через внешнего провайдера" }).click();
  await expect(page.getByText("Повтор: уже учтён.").last()).toBeVisible();

  const layout = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
    mainWidth: document.querySelector("main")?.getBoundingClientRect().width ?? 0,
  }));
  expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);
  expect(layout.mainWidth).toBeGreaterThan(0);
  expect(layout.mainWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
});

test("critical journey can be completed from the keyboard", async ({ page }) => {
  await page.goto("/");
  const request = page.getByLabel("С чем нужна помощь");
  await request.focus();
  await page.keyboard.type("Не могу спать, бессонница");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Разобрать запрос" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("checkbox", { name: "Сон" })).toBeChecked();
  const confirm = page.getByRole("button", { name: "Подтвердить и показать специалистов" });
  await confirm.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { level: 3, name: "Марина Лебедева" })).toBeVisible();
});
