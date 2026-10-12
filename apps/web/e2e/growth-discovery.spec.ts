import { expect, test } from "@playwright/test";

test("public topic navigation preserves critical entry layout and accessibility", async ({ page }) => {
  await page.goto("/");
  const chips = page.locator(".direction-grid a");
  await expect(chips).toHaveCount(4);
  const targets = ["/help/trevoga", "/help/son", "/help/otnosheniya", "/help/kariera"];
  for (let index = 0; index < targets.length; index++) {
    const chip = chips.nth(index);
    await expect(chip).toHaveAttribute("href", targets[index]);
    await expect(chip).toHaveCSS("text-decoration-line", "none");
    await expect(chip).toHaveCSS("background-color", "rgb(238, 243, 237)");
    await expect(chip).toHaveCSS("padding-top", "9px");
    await expect(chip).toHaveCSS("padding-bottom", "9px");
  }

  // The same published links are keyboard reachable, unlike a decorative span.
  await chips.first().focus();
  await expect(chips.first()).toBeFocused();
  await expect(chips.first()).toHaveCSS("outline-style", "solid");
});

test("public demand page fails closed without qualified live supply", async ({ page }) => {
  await page.goto("/help/son");
  await expect(page.getByRole("heading", { name: "Поиск специалиста" })).toBeVisible();
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
  await expect(page.getByRole("link", { name: "Начать общий подбор" })).toHaveAttribute("href", "/#start");
  await expect(page.locator(".how-grid article")).toHaveCount(0);
});
