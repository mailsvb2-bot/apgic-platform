import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("public HTML is not served with an immutable one-year cache", async ({ request }) => {
  for (const path of ["/", "/specialist", "/organization", "/update"]) {
    const response = await request.get(path);
    expect(response.ok()).toBeTruthy();
    const cacheControl = response.headers()["cache-control"] ?? "";
    expect(cacheControl).not.toContain("s-maxage=31536000");
    expect(cacheControl.toLowerCase()).toContain("no-store");
  }
});

test("browser session reuses one canonical Identity across HelpIntents", async ({ page, context }) => {
  await page.goto("/");

  const identities = await page.evaluate(async () => {
    async function create(freeText: string) {
      const response = await fetch("/v1/help-intents", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ free_text: freeText }),
      });
      if (!response.ok) {
        throw new Error("help intent create failed");
      }
      return response.json() as Promise<{ id: string; client_identity_id: string }>;
    }

    const first = await create("первый запрос");
    const second = await create("второй запрос");
    return {
      firstIntent: first.id,
      secondIntent: second.id,
      firstIdentity: first.client_identity_id,
      secondIdentity: second.client_identity_id,
    };
  });

  expect(identities.firstIntent).not.toBe(identities.secondIntent);
  expect(identities.firstIdentity).toBeTruthy();
  expect(identities.firstIdentity).toBe(identities.secondIdentity);

  const cookies = await context.cookies();
  const session = cookies.find((cookie) => cookie.name === "__Host-apgic_session");
  expect(session).toBeTruthy();
  expect(session?.httpOnly).toBeTruthy();
  expect(session?.secure).toBeTruthy();
  expect(session?.sameSite).toBe("Lax");
});

test("help intent journey stays usable and accessible", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  const journeyProgress = page.getByRole("navigation", { name: "Этапы записи" });
  await expect(journeyProgress).toBeVisible();
  const currentJourneyStep = journeyProgress.locator('[aria-current="step"]');
  await expect(currentJourneyStep).toBeVisible();
  await expect(currentJourneyStep).toContainText("Запрос");
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
  await expect(page.getByRole("button", { name: "Сбросить поисковый индекс" })).toBeHidden();
  await page.getByText("Проверка каталога и поисковой проекции").click();
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
  await expect(page.getByText(/^Бронь \S+ в состоянии HELD\./)).toBeVisible();
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
  await expect(page.getByRole("button", { name: "Завершить без доказательства" })).toBeHidden();
  await page.getByText("Служебные проверки безопасности").click();
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
  await expect(page.getByText("Сырая запись в деле: нет.")).toBeVisible();
  await page.getByRole("button", { name: "Передать сырую запись в рост" }).click();
  await expect(page.locator(".alert")).toContainText("согласия");
  await page.getByRole("button", { name: "Отменить бронь через внешнего провайдера" }).click();
  await expect(page.getByRole("heading", { name: "Бронь отменена" })).toBeVisible();
  await expect(page.getByText("CANCELLED")).toBeVisible();
  await expect(page.getByText("APGIC возвращает деньги: нет.")).toBeVisible();
  await page.getByRole("button", { name: "Отменить бронь через внешнего провайдера" }).click();
  await expect(page.getByText("Повтор: уже учтён.").last()).toBeVisible();
  await page.getByText("Управление учётной записью").click();
  await page.getByRole("button", { name: "Удалить учётную запись" }).click();
  await expect(page.getByText("PARTIALLY_RETAINED_WITH_REASON")).toBeVisible();
  await expect(page.getByText("Деактивация: нет.")).toBeVisible();
  await expect(page.getByText("Запись учёта сохранена: да.")).toBeVisible();
  await expect(page.getByText("APGIC уничтожает запись учёта: нет.")).toBeVisible();
  await page.getByRole("button", { name: "Удалить учётную запись" }).click();
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

test("replacing a help request invalidates stale matches even on API failure", async ({ page }) => {
  await page.goto("/");
  const request = page.getByLabel("С чем нужна помощь");
  await request.fill("Мне сложно уснуть, бессонница");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await expect(page.getByRole("checkbox", { name: "Сон" })).toBeChecked();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();
  await expect(page.getByRole("heading", { level: 3, name: "Марина Лебедева" })).toBeVisible();

  await page.route("**/v1/help-intents", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ message_safe: "Временно недоступно" }),
      });
      return;
    }
    await route.continue();
  });

  await request.fill("Новый запрос о карьере");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await expect(page.locator(".journey-alert")).toContainText("Временно недоступно");
  await expect(page.getByRole("heading", { level: 3, name: "Марина Лебедева" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Подтвердить и показать специалистов" })).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Этапы записи" }).locator('[aria-current="step"]')).toContainText("Запрос");
});

test("failed replacement slot hold hides previous checkout instead of mixing bookings", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("С чем нужна помощь").fill("Проблемы со сном, бессонница");
  await page.getByRole("button", { name: "Разобрать запрос" }).click();
  await expect(page.getByRole("checkbox", { name: "Сон" })).toBeChecked();
  await page.getByRole("button", { name: "Подтвердить и показать специалистов" }).click();
  await page.getByRole("button", { name: "Выбрать время у Марина Лебедева" }).click();
  const slotButtons = page.getByRole("button", { name: /Удержать слот/ });
  await expect(slotButtons.first()).toBeVisible();
  let chosenSlot = -1;
  const count = await slotButtons.count();
  for (let index = 0; index < count; index += 1) {
    await slotButtons.nth(index).click();
    const success = page.getByRole("heading", { name: "Слот удерживается" });
    await expect(success.or(page.locator(".journey-alert"))).toBeVisible();
    if (await success.isVisible()) {
      chosenSlot = index;
      break;
    }
  }
  expect(chosenSlot).toBeGreaterThanOrEqual(0);
  await expect(page.getByRole("button", { name: "Выбрать Карта через внешнего провайдера" })).toBeVisible();

  await page.route("**/v1/slot-holds", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ message_safe: "Слот недоступен" }) });
      return;
    }
    await route.continue();
  });
  await slotButtons.nth(chosenSlot).click();
  await expect(page.locator(".journey-alert")).toContainText("Слот недоступен");
  await expect(page.getByRole("heading", { name: "Слот удерживается" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Выбрать Карта через внешнего провайдера" })).toHaveCount(0);
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

test("critical journey survives 200% text scaling", async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => {
    document.documentElement.style.fontSize = "200%";
  });

  const request = page.getByLabel("С чем нужна помощь");
  await expect(request).toBeVisible();
  await request.fill("Не могу спать, бессонница");

  const primary = page.getByRole("button", { name: "Разобрать запрос" });
  await expect(primary).toBeVisible();
  await primary.click();
  await expect(page.getByRole("button", { name: "Подтвердить и показать специалистов" })).toBeVisible();

  const layout = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
  }));
  expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);
});


test("critical journey boots when crypto.randomUUID is unavailable", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(globalThis.crypto, "randomUUID", {
      value: undefined,
      configurable: true,
    });
  });
  await page.goto("/");
  await expect(page.getByLabel("С чем нужна помощь")).toBeVisible();
  await expect(page.getByRole("button", { name: "Разобрать запрос" })).toBeVisible();
});


test("specialist onboarding preserves evidence and publish boundaries", async ({ page }) => {
  type Profile = {
    id: string;
    identity_id: string;
    display_name: string;
    profession_code: string;
    profile_complete: boolean;
    review_state: string;
    capabilities: Array<{
      topic_id: string;
      evidence_state: string;
      verification_state: string;
      evidence_refs: string[];
    }>;
    evidence: Array<{
      id: string;
      topic_id: string;
      kind: string;
      reference: string;
      state: string;
      submitted_at: string;
    }>;
    published_topics: string[];
  };

  let profile: Profile | null = null;
  await page.route("**/v1/specialist/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const json = request.postDataJSON?.() as Record<string, string> | undefined;

    if (path === "/v1/specialist/profile" && request.method() === "GET") {
      if (!profile) {
        await route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({
          code: "SPECIALIST_PROFILE_NOT_FOUND",
          message_safe: "Профессиональный профиль ещё не создан.",
          correlation_id: "e2e",
          retryable: false,
        }) });
        return;
      }
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(profile) });
      return;
    }

    if (path === "/v1/specialist/profile" && request.method() === "PUT") {
      profile = {
        id: "specialist-e2e",
        identity_id: "identity-e2e",
        display_name: json?.display_name || "",
        profession_code: json?.profession_code || "",
        profile_complete: true,
        review_state: "PENDING",
        capabilities: [],
        evidence: [],
        published_topics: [],
      };
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(profile) });
      return;
    }

    if (path === "/v1/specialist/capabilities" && request.method() === "POST" && profile) {
      profile.capabilities = [{
        topic_id: json?.topic_id || "anxiety",
        evidence_state: "SELF_DECLARED",
        verification_state: "NOT_APPLICABLE",
        evidence_refs: [],
      }];
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(profile) });
      return;
    }

    if (path === "/v1/specialist/evidence" && request.method() === "POST" && profile) {
      const reference = json?.reference || "";
      profile.review_state = "MANUAL_REVIEW";
      profile.capabilities = [{
        topic_id: json?.topic_id || "anxiety",
        evidence_state: "DOCUMENT_SUPPORTED",
        verification_state: "PENDING",
        evidence_refs: [reference],
      }];
      profile.evidence = [{
        id: "evidence-e2e",
        topic_id: json?.topic_id || "anxiety",
        kind: json?.kind || "DIPLOMA",
        reference,
        state: "SUBMITTED",
        submitted_at: "2026-09-28T20:00:00Z",
      }];
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(profile) });
      return;
    }

    if (path === "/v1/specialist/publish" && request.method() === "POST") {
      await route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({
        allowed: false,
        reason_codes: ["PUBLISH_REVIEW_INCOMPLETE"],
        published_topics: [],
        policy_version: "qualification-v1",
      }) });
      return;
    }

    await route.fulfill({ status: 404, contentType: "application/json", body: "{}" });
  });

  await page.goto("/specialist");
  await expect(page.getByRole("heading", { level: 1, name: /Работайте с клиентами/ })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Создайте профессиональный профиль" })).toBeVisible();

  await page.getByLabel("Имя для публичного профиля").fill("Анна Тестова");
  await page.getByLabel("Профессия").selectOption("PSYCHOLOGIST");
  await page.getByRole("button", { name: "Сохранить профиль" }).click();
  await expect(page.getByText("Профиль сохранён. Теперь добавьте направления работы.")).toBeVisible();

  await page.getByLabel("Направление").selectOption("anxiety");
  await page.getByRole("button", { name: "Добавить направление" }).click();
  await expect(page.getByText("SELF_DECLARED · NOT_APPLICABLE")).toBeVisible();

  await page.getByLabel("Ссылка или номер подтверждения").fill("document:e2e-diploma");
  await page.getByRole("button", { name: "Передать на проверку" }).click();
  await expect(page.getByText("DOCUMENT_SUPPORTED · PENDING")).toBeVisible();
  await expect(page.getByText("SUBMITTED · Тревога и стресс")).toBeVisible();
  await expect(page.getByText(/Review профиля:/)).toContainText("MANUAL_REVIEW");

  await page.getByRole("button", { name: "Проверить и опубликовать" }).click();
  await expect(page.getByRole("status")).toContainText("PUBLISH_REVIEW_INCOMPLETE");
  await expect(page.getByText(/Опубликовано:/)).toContainText("пока ничего");

  const layout = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
  }));
  expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth + 1);

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
});


test("organization workspace uses real organization lifecycle endpoints", async ({ page }) => {
  type Snapshot = {
    id: string;
    name: string;
    status: string;
    directions: Array<{
      id: string;
      organization_id: string;
      name: string;
      direction_type: string;
      status: string;
    }>;
  };
  type Product = {
    id: string;
    name: string;
    status: "DRAFT" | "PUBLISHED";
    owner_type: "ORGANIZATION";
    owner_id: string;
    commercial_owner_ref: string;
    author_refs: string[];
    revenue_beneficiary_ref: string;
    organization_direction_id: string;
    published_at?: string;
  };
  let snapshot: Snapshot | null = null;
  let products: Product[] = [];

  await page.route("**/v1/organizations**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const body = request.postDataJSON?.() as Record<string, string> | undefined;

    if (path === "/v1/organizations" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ organizations: snapshot ? [snapshot] : [] }),
      });
      return;
    }

    if (path === "/v1/organizations" && request.method() === "POST") {
      snapshot = { id: "org-e2e", name: body?.name || "", status: "ACTIVE", directions: [] };
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(snapshot) });
      return;
    }
    if (path === "/v1/organizations/org-e2e/products" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ products }),
      });
      return;
    }
    if (path === "/v1/organizations/org-e2e/products" && request.method() === "POST" && snapshot) {
      const requestBody = request.postDataJSON() as {
        name: string;
        direction_id: string;
        commercial_owner_ref: string;
        author_refs: string[];
        revenue_beneficiary_ref: string;
      };
      const product: Product = {
        id: "product-e2e",
        name: requestBody.name,
        status: "DRAFT",
        owner_type: "ORGANIZATION",
        owner_id: snapshot.id,
        commercial_owner_ref: requestBody.commercial_owner_ref,
        author_refs: requestBody.author_refs,
        revenue_beneficiary_ref: requestBody.revenue_beneficiary_ref,
        organization_direction_id: requestBody.direction_id,
      };
      products = [product];
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(product) });
      return;
    }
    if (
      path === "/v1/organizations/org-e2e/products/product-e2e/publish" &&
      request.method() === "POST" &&
      products.length
    ) {
      products = [{ ...products[0], status: "PUBLISHED", published_at: "2026-09-29T18:00:00Z" }];
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(products[0]) });
      return;
    }
    if (path === "/v1/organizations/org-e2e/directions" && request.method() === "POST" && snapshot) {
      snapshot.directions.push({
        id: "direction-e2e",
        organization_id: snapshot.id,
        name: body?.name || "",
        direction_type: body?.direction_type || "GENERAL",
        status: "ACTIVE",
      });
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(snapshot) });
      return;
    }
    if (path === "/v1/organizations/org-e2e/directions/direction-e2e/archive" && request.method() === "POST" && snapshot) {
      snapshot.directions = snapshot.directions.map((direction) =>
        direction.id === "direction-e2e" ? { ...direction, status: "ARCHIVED" } : direction
      );
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(snapshot) });
      return;
    }
    await route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ message_safe: "Не найдено" }) });
  });

  await page.goto("/organization");
  await expect(page.getByRole("heading", { level: 1, name: /Управляйте организацией/ })).toBeVisible();
  await page.getByLabel("Название организации").fill("Центр развития");
  await page.getByRole("button", { name: "Создать организацию" }).click();
  await expect(page.getByRole("heading", { name: "Центр развития" })).toBeVisible();
  await expect(page.getByRole("status")).toContainText("активный владелец");

  await page.getByLabel("Название направления").fill("Психологическая помощь");
  await page.getByLabel("Тип направления").selectOption("SERVICE");
  await page.getByRole("button", { name: "Добавить направление" }).click();
  const activeDirectionItem = page.locator(".organization-directions li").filter({ hasText: "Психологическая помощь" }).filter({ hasText: "SERVICE · ACTIVE" });
  await expect(activeDirectionItem).toHaveCount(1);
  await expect(activeDirectionItem).toBeVisible();

  await page.getByLabel("Название продукта").fill("Первичная консультация");
  await page.getByLabel("Направление продукта").selectOption("direction-e2e");
  await page.getByLabel("Коммерческий владелец").fill("organization/org-e2e");
  await page.getByLabel("Авторы").fill("identity/specialist-e2e");
  await page.getByLabel("Получатель выручки").fill("identity/specialist-e2e");
  await page.getByRole("button", { name: "Создать черновик продукта" }).click();
  await expect(page.getByText("DRAFT · направление direction-e2e")).toBeVisible();
  await expect(page.getByText("Коммерческий владелец: organization/org-e2e")).toBeVisible();
  await expect(page.getByText("Авторы: identity/specialist-e2e")).toBeVisible();
  await expect(page.getByText("Получатель выручки: identity/specialist-e2e")).toBeVisible();

  await page.getByRole("button", { name: "Опубликовать" }).click();
  const publishedProductItem = page.locator(".organization-products li").filter({ hasText: "PUBLISHED · направление direction-e2e" });
  await expect(publishedProductItem).toHaveCount(1);
  await expect(publishedProductItem.locator(".product-published")).toHaveText("Опубликован");
  await expect(page.getByRole("status")).toContainText("опубликован");

  await page.getByRole("button", { name: "Архивировать" }).click();
  await expect(page.getByText("SERVICE · ARCHIVED")).toBeVisible();
  await expect(page.getByRole("status")).toContainText("История сохранена");

  await page.reload();
  await expect(page.getByRole("heading", { name: "Центр развития" })).toBeVisible();
  await expect(page.getByText("SERVICE · ARCHIVED")).toBeVisible();
  const reloadedPublishedProductItem = page.locator(".organization-products li").filter({ hasText: "PUBLISHED · направление direction-e2e" });
  await expect(reloadedPublishedProductItem).toHaveCount(1);
  await expect(reloadedPublishedProductItem.locator(".product-published")).toHaveText("Опубликован");
  await expect(page.getByRole("status")).toContainText("Организации загружены");

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
});


test("canonical web deep-link fallback revalidates before opening resource route", async ({ page }) => {
  const token = "v1.c2VhbGVkLWFlYWQtYmxvYg";
  const canonicalPath = "/specialists/e2e-specialist";
  await page.route("**/v1/mobile/deep-links/resolve?token=*", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        decision: "ALLOW",
        reason_code: "DEEPLINK_ALLOWED",
        canonical_path: canonicalPath,
        canonical_web_fallback: `https://apgic.ru${canonicalPath}`,
        expires_at: "2026-10-01T12:15:00Z",
      }),
    });
  });

  await page.goto(`/l/${token}`);
  await expect(page).toHaveURL(/\/specialists\/e2e-specialist$/);
  await expect(page.getByRole("heading", { name: "Специалист" })).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("Ресурс подтверждён сервером.");
  await expect(page.getByText("Идентификатор: e2e-specialist")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("status")).toHaveText("Ресурс подтверждён сервером.");
});

test("canonical resource route fails closed when revalidation does not match path", async ({ page }) => {
  const token = "v1.c2VhbGVkLWFlYWQtYmxvYg";
  await page.route("**/v1/mobile/deep-links/resolve?token=*", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        decision: "ALLOW",
        reason_code: "DEEPLINK_ALLOWED",
        canonical_path: "/bookings/other-booking",
        canonical_web_fallback: "https://apgic.ru/bookings/other-booking",
        expires_at: "2026-10-01T12:15:00Z",
      }),
    });
  });

  await page.goto("/bookings/booking-1");
  await page.evaluate((value) => {
    sessionStorage.setItem("apgic:deeplink:/bookings/booking-1", value);
  }, token);
  await page.reload();
  await expect(page.locator("main").getByRole("alert")).toContainText("доступ к ресурсу не подтверждён");
});


test("mobile forced-update destination is safe and does not invent store availability", async ({ page }) => {
  await page.goto("/update");
  await expect(page.getByRole("heading", { level: 1, name: "Обновите приложение APGIC" })).toBeVisible();
  await expect(page.getByText("Не удаляйте приложение и не повторяйте оплату")).toBeVisible();
  await expect(page.getByText("официальный магазин или канал", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: "Вернуться на сайт" })).toHaveAttribute("href", "/");
});
