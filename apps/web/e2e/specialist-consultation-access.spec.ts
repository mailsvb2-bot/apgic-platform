import { expect, test } from "@playwright/test";

const id = "00000000-0000-4000-8000-000000000901";

test("specialist can resume only an authenticated, server-authorized consultation", async ({page}) => {
  let mode: "forbidden" | "client" | "specialist" = "forbidden";
  let reads = 0;
  await page.route(`**/v1/consultations/${id}/ready`, async route => {
    reads++;
    if (mode === "forbidden") {
      await route.fulfill({status:404,contentType:"application/json",body:"{}"});
      return;
    }
    await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
      booking_id:id,role:mode === "client" ? "CLIENT" : "SPECIALIST",
      booking_state:"CONFIRMED",opens_at:"2030-10-12T10:00:00Z",closes_at:"2030-10-12T11:00:00Z",
    })});
  });
  await page.goto("/specialist");
  const input=page.getByLabel("Идентификатор записи");
  await input.fill("not-a-booking");
  await page.getByRole("button",{name:"Проверить доступ"}).click();
  await expect(page.getByText("Введите корректный идентификатор записи.")).toBeVisible();
  expect(reads).toBe(0);
  await input.fill(id);
  await page.getByRole("button",{name:"Проверить доступ"}).click();
  await expect(page.getByText("Запись не найдена или доступ к ней отсутствует.")).toBeVisible();
  await expect(page.getByRole("link",{name:"Открыть рабочее место консультации"})).toHaveCount(0);
  mode="client";
  await page.getByRole("button",{name:"Проверить доступ"}).click();
  await expect(page.getByText("Сервер не подтвердил доступ специалиста к этой записи.")).toBeVisible();
  await expect(page.getByRole("link",{name:"Открыть рабочее место консультации"})).toHaveCount(0);
  mode="specialist";
  await page.getByRole("button",{name:"Проверить доступ"}).click();
  const link=page.getByRole("link",{name:"Открыть рабочее место консультации"});
  await expect(link).toHaveAttribute("href",`/specialist/consultations/${id}`);
  await input.fill("changed");
  await expect(link).toHaveCount(0);
});
