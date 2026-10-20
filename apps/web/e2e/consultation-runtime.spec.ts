import { expect, test } from "@playwright/test";

// Browser proof is a simulated API contract. Real provider media and PostgreSQL
// ownership are separately verified by R3 domain and integration tests.
test("confirmed booking enables readiness, fail-closed room entry and verified personal outcome", async ({ page }) => {
  const id = "00000000-0000-4000-8000-000000000901";
  let ready = false;
  let providerConfigured = false;
  let stableJoinKey: string | null = null;
  let joinRequests = 0;
  let completed = false;
  let outcomeCount = 0;
  const booking = {
    booking_id: id, hold_id: "hold-runtime-e2e", state: "CONFIRMED",
    hold_expires_at: "2026-10-12T10:00:00Z",
    starts_at: "2026-10-12T10:15:00Z", ends_at: "2026-10-12T11:15:00Z",
  };
  await page.route(`**/v1/bookings/${id}`, async route => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(booking) });
  });
  // The browser fixture simulates a canonical server-confirmed PSP capture.
  // Production access cannot be inferred from CONFIRMED booking alone.
  await page.route(`**/v1/bookings/${id}/order`, async route => {
    await route.fulfill({status:200, contentType:"application/json", body:JSON.stringify({
      order_id:"order-consultation-e2e", booking_id:id, booking_state:"CONFIRMED",
      payment_state:"CAPTURED", amount_minor:150000, currency:"RUB",
      provider_id:"external-psp", payment_method:"SBP",
    })});
  });
  await page.route(`**/v1/consultations/${id}/ready`, async route => {
    const request = route.request();
    if (request.method() === "POST") {
      expect(request.headers()["x-apgic-request"]).toBe("consultation-v1");
      ready = true;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      booking_id:id, role:"CLIENT", booking_state:"CONFIRMED",
      opens_at:"2026-10-12T10:00:00Z",closes_at:"2026-10-12T11:15:00Z",
      ...(ready ? {client_ready_at:"2026-10-12T10:03:00Z", specialist_ready_at:"2026-10-12T10:04:00Z"} : {}),
    }) });
  });
  await page.route(`**/v1/consultations/${id}/result`, async route => {
    if (!completed) return route.fulfill({status:404,body:"{}"});
    return route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
      booking_id:id,state:"COMPLETED",provider_instance_id:"communication-1",
      completion_evidence_ref:"provider-signed/complete-1",
    })});
  });
  await page.route(`**/v1/consultations/${id}/join`, async route => {
    const request = route.request();
    expect(request.method()).toBe("POST");
    expect(request.headers()["idempotency-key"]).toMatch(/^consultation-join:/);
    const key = request.headers()["idempotency-key"];
    joinRequests++;
    if (stableJoinKey === null) stableJoinKey = key;
    else expect(key).toBe(stableJoinKey);
    expect(request.headers()["x-apgic-request"]).toBe("consultation-v1");
    await route.fulfill({status:providerConfigured?200:503,contentType:"application/json",
      body:JSON.stringify(providerConfigured?{
        booking_id:id,provider_instance_id:"communication-1",
        join_uri:"https://media.example/room/one?token=ephemeral",
        credential_scope:"ROOM_JOIN",credential_expires_at:"2030-10-12T11:15:00Z",
        reason_code:"COMM_JOIN_ALLOWED",
      }:{error:{code:"COMM_PROVIDER_UNAVAILABLE"}})});
  });
  await page.route(`**/v1/consultations/${id}/outcome`, async route => {
    const request = route.request();
    if (request.method()==="GET") return route.fulfill({status:404,body:"{}"});
    outcomeCount++;
    const body=request.postDataJSON() as {progress:number;match_satisfaction:number;idempotency_key:string};
    expect(body.idempotency_key.length).toBeGreaterThanOrEqual(16);
    await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
      booking_id:id,progress:body.progress,match_satisfaction:body.match_satisfaction,
      reported_at:"2026-10-12T11:20:00Z",
    })});
  });
  await page.goto(`/bookings/${id}`);
  const region=page.getByRole("region",{name:"Готовность участников и личный отклик"});
  await expect(region.getByText("Клиент: готовность не подтверждена.",{exact:false})).toBeVisible();
  await expect(region.getByRole("button",{name:"Получить вход в консультацию"})).toHaveCount(0);
  await expect(region.getByRole("heading",{name:"Мой отклик после консультации"})).toHaveCount(0);
  await region.getByRole("button",{name:"Я готов"}).click();
  await expect(region.getByRole("button",{name:"Получить вход в консультацию"})).toBeVisible();
  await region.getByRole("button",{name:"Получить вход в консультацию"}).click();
  await expect(region.getByText(/Вход пока не подтверждён/)).toBeVisible();
  await expect(region.getByRole("link",{name:"Открыть защищённую комнату"})).toHaveCount(0);
  providerConfigured = true;
  await region.getByRole("button",{name:"Получить вход в консультацию"}).click();
  expect(joinRequests).toBe(2);
  const link = region.getByRole("link",{name:"Открыть защищённую комнату"});
  await expect(link).toHaveAttribute("href",/https:\/\/media\.example\/room\/one/);
  await expect(link).toHaveAttribute("rel",/noreferrer/);
  expect(outcomeCount).toBe(0);
  completed = true;
  await page.reload();
  const resumed = page.getByRole("region",{name:"Готовность участников и личный отклик"});
  await expect(resumed.getByRole("heading",{name:"Мой отклик после консультации"})).toBeVisible();
  await resumed.getByLabel("Изменение состояния").selectOption("1");
  await resumed.getByLabel("Насколько подошёл специалист").selectOption("5");
  await resumed.getByRole("button",{name:"Сохранить мой отклик"}).click();
  await expect(resumed.getByText("Ваш отклик сохранён.")).toBeVisible();
  expect(outcomeCount).toBe(1);
  // The completed consultation must lead to the same owner's server-backed
  // personal history, not a browser-local success screen.
  await page.route("**/v1/client/personal", async route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      identity_id: "00000000-0000-4000-8000-000000000902",
      as_of: "2026-10-12T12:00:00Z",
      preferences: { display_name: "Клиент", locale: "ru", time_zone: "Europe/Moscow" },
      upcoming: [], requests: [],
      history: [{
        booking_id: id, state: "COMPLETED",
        starts_at: "2026-10-12T10:15:00Z", ends_at: "2026-10-12T11:15:00Z",
        specialist_name: "Ирина Тестовая", specialist_profession: "PSYCHOLOGIST",
        consultation_state: "COMPLETED",
      }],
    }),
  }));
  await resumed.getByRole("link", { name: "Посмотреть завершённую консультацию в личном кабинете" }).click();
  await expect(page).toHaveURL(new RegExp("/personal#personal-history$"));
  await expect(page.getByRole("heading", { name: "История встреч" })).toBeVisible();
  await expect(page.getByText("Ирина Тестовая")).toBeVisible();
  await expect(page.getByRole("link", { name: "Детали записи" })).toHaveAttribute("href", `/bookings/${id}`);
});

test("verified historical completion remains reachable when active payment entitlement cannot be established", async ({ page }) => {
  const id = "00000000-0000-4000-8000-000000000911";
  await page.route(`**/v1/bookings/${id}`, route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      booking_id: id, hold_id: "hold-history-e2e", state: "COMPLETED",
      hold_expires_at: "2026-10-12T10:00:00Z",
      starts_at: "2026-10-12T10:15:00Z", ends_at: "2026-10-12T11:15:00Z",
    }),
  }));
  await page.route(`**/v1/bookings/${id}/order`, route => route.fulfill({ status: 503, body: "{}" }));
  await page.route(`**/v1/consultations/${id}/result`, route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      booking_id: id, state: "COMPLETED", provider_instance_id: "communication-1",
      completion_evidence_ref: "provider-signed/finished-911",
    }),
  }));
  await page.route(`**/v1/client/personal`, route => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({
      identity_id: "00000000-0000-4000-8000-000000000912",
      as_of: "2026-10-12T12:00:00Z",
      preferences: { display_name: "Клиент", locale: "ru", time_zone: "Europe/Moscow" },
      upcoming: [], requests: [],
      history: [{
        booking_id: id, state: "COMPLETED",
        starts_at: "2026-10-12T10:15:00Z", ends_at: "2026-10-12T11:15:00Z",
        specialist_name: "Проверенный специалист", specialist_profession: "PSYCHOLOGIST",
        consultation_state: "COMPLETED",
      }],
    }),
  }));
  await page.goto(`/bookings/${id}`);
  await expect(page.getByText(/Платёжные сведения не удалось проверить/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Я готов" })).toHaveCount(0);
  const result = page.getByRole("region", { name: "Результат консультации" });
  await result.getByRole("button", { name: "Проверить результат консультации" }).click();
  await expect(result.getByText("Консультация завершена по подтверждённому событию провайдера.")).toBeVisible();
  await result.getByRole("link", { name: "Посмотреть консультацию в истории личного кабинета" }).click();
  await expect(page).toHaveURL(new RegExp("/personal#personal-history$"));
  await expect(page.getByText("Проверенный специалист")).toBeVisible();
});
