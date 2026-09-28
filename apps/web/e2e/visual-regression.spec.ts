import { expect, test } from "@playwright/test";

test("critical entry matches the approved visual baseline", async ({ page }, testInfo) => {
  test.skip(process.platform !== "linux", "Canonical visual baselines are captured on the Linux CI renderer.");

  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("button", { name: "Разобрать запрос" })).toBeVisible();

  await expect(page).toHaveScreenshot(`${testInfo.project.name}-critical-entry.png`, {
    animations: "disabled",
    caret: "hide",
    fullPage: true,
    maxDiffPixelRatio: 0.001,
  });
});
