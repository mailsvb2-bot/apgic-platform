import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("foundation surface remains usable and accessible", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1, name: "APGIC Platform" })).toBeVisible();
  await expect(page.getByRole("list", { name: "Поддерживаемые поверхности" })).toBeVisible();

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
