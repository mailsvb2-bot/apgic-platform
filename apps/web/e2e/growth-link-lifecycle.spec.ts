import { expect, test } from "@playwright/test";

const token = "g1.abc.def";
const path = "/g/" + token;
const offer = {
  id: "offer-1", topic_id: "sleep", state: "ACTIVE",
  price_minor: 200000, currency: "RUB",
};
const activeLink = {
  id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  offer_id: offer.id, topic_id: "sleep", source: "SPECIALIST_SOURCED",
  state: "ACTIVE", expires_at: "2099-11-01T12:00:00Z", path,
};
test("professional owner resumes, reuses and revokes a signed distribution link", async ({ page }) => {
  let revoked = false;
  let issued = 0;
  await page.route("**/v1/specialist/offers", async (route) => {
    await route.fulfill({ json: { offers: [offer] } });
  });
  await page.route("**/v1/specialist/growth-report", async (route) => {
    await route.fulfill({ json: { items: [] } });
  });
  await page.route("**/v1/specialist/growth-links", async (route) => {
    if (route.request().method() === "POST") {
      issued++;
      await route.fulfill({ status: 201, json: activeLink });
      return;
    }
    await route.fulfill({
      json: { links: [revoked ? { ...activeLink, state: "REVOKED", path: "" } : activeLink] },
    });
  });
  await page.route("**/v1/specialist/growth-links/*/revoke", async (route) => {
    revoked = true;
    await route.fulfill({ json: { state: "REVOKED" } });
  });

  await page.goto("/specialist/growth");
  await expect(page.getByRole("heading", { name: "Ссылки и история" })).toBeVisible();
  await expect(page.locator(`a[href="${path}"]`)).toBeVisible();
  await page.reload();
  await expect(page.locator(`a[href="${path}"]`)).toBeVisible();

  await page.getByRole("button", { name: "Получить ссылку для предложения" }).click();
  await expect.poll(() => issued).toBe(1);
  await expect(page.locator(`a[href="${path}"]`)).toBeVisible();
  await expect(page.getByRole("button", { name: "Поделиться" })).toBeVisible();
  await page.getByRole("button", { name: "Отозвать" }).click();
  await expect(page.getByRole("region", { name: "Мои ссылки распространения" }).getByText(/^Статус: Отозвана ·/)).toBeVisible();
  await expect(page.locator(`a[href="${path}"]`)).toHaveCount(0);
  await page.reload();
  await expect(page.locator(`a[href="${path}"]`)).toHaveCount(0);
});

test("failed storage does not expose stale specialist links", async ({ page }) => {
  await page.route("**/v1/specialist/offers", async (route) => {
    await route.fulfill({ json: { offers: [offer] } });
  });
  await page.route("**/v1/specialist/growth-links", async (route) => {
    await route.fulfill({
      status: 503, json: { message_safe: "Не удалось получить проверенные ссылки." },
    });
  });
  await page.route("**/v1/specialist/growth-report", async (route) => {
    await route.fulfill({ json: { items: [] } });
  });
  await page.goto("/specialist/growth");
  await expect(page.getByRole("alert", { name: "Не удалось получить проверенные ссылки." })).toBeVisible();
  await expect(page.locator('a[href^="/g/"]')).toHaveCount(0);
});
