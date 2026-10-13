import { expect, test } from "@playwright/test";

// A browser-owned hold ID is merely a pointer for an authenticated owner
// read. Restoring a CONFIRMED booking never authorizes a consultation without
// checking the order against the canonical financial readback.
test("restored booking does not imply confirmed payment or consultation access", async ({page}) => {
  const old = "00000000-0000-4000-8000-00000000a171";
  const booking = "00000000-0000-4000-8000-00000000b171";
  await page.addInitScript((id) => {
    sessionStorage.setItem("apgic:current-hold-id", id);
  }, old);
  await page.route(`**/v1/slot-holds/${old}`, route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      id: old, booking_id: booking, state: "CONSUMED",
      booking_state: "CONFIRMED", expires_at: "2030-01-01T20:00:00Z",
      reason_code: "BOOKING_CONFIRMED",
    }),
  }));
  await page.goto("/");
  const restored = page.getByRole("region", {name: "Восстановленная запись"});
  await expect(restored).toBeVisible();
  await expect(restored).toContainText("не самостоятельное доказательство списания");
  await expect(page.getByRole("region", {name: "Результат консультации"})).toHaveCount(0);
  await expect(restored.getByRole("link", {name: "Проверить запись и оплату"}))
    .toHaveAttribute("href", `/bookings/${booking}`);
});

test("old restore 404 cannot erase a newly acquired hold after user changes journey", async ({page}) => {
  const old = "00000000-0000-4000-8000-00000000a172";
  let begun: (() => void) | undefined;
  let release: (() => void) | undefined;
  const requested = new Promise<void>(resolve => { begun = resolve; });
  const released = new Promise<void>(resolve => { release = resolve; });
  await page.addInitScript(id => {
    sessionStorage.setItem("apgic:current-hold-id", id);
  }, old);
  await page.route(`**/v1/slot-holds/${old}`, async route => {
    begun?.();
    await released;
    await route.fulfill({status: 404, contentType: "application/json", body: "{}"});
  });
  await page.goto("/");
  await requested;
  await page.getByLabel("С чем нужна помощь").fill("Мне тревожно перед выступлениями");
  await page.getByRole("button", {name: "Разобрать запрос"}).click();
  await page.getByRole("button", {name: "Подтвердить и показать специалистов"}).click();
  await page.getByRole("button", {name: "Выбрать время у Марина Лебедева"}).click();
  await page.getByRole("button", {name: /Удержать слот/}).first().click();
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();
  const current = await page.evaluate(() => sessionStorage.getItem("apgic:current-hold-id"));
  expect(current).toBeTruthy();
  expect(current).not.toBe(old);
  const finished = page.waitForResponse(res => res.url().endsWith("/v1/slot-holds/" + old));
  release?.();
  await finished;
  await expect.poll(async () => page.evaluate(() =>
    sessionStorage.getItem("apgic:current-hold-id"))).toBe(current);
  await expect(page.getByRole("heading", {name: "Слот удерживается"})).toBeVisible();
  await expect(page.getByRole("region", {name: "Результат консультации"})).toHaveCount(0);
});
