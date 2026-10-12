import {expect, test} from "@playwright/test";

test("R9 public event route remains truthful when feature gate is closed", async ({page}) => {
  await page.goto("/live-events");
  await expect(page.getByRole("heading", {name: "Встречи, вебинары и мастер-классы"})).toBeVisible();
  await expect(page.getByRole("alert")).toContainText("Раздел пока не подключён.");
  await expect(page.getByText("Сейчас нет опубликованных мероприятий.")).toHaveCount(0);
});

test("R9 organizer route is discoverable without inventing media readiness", async ({page}) => {
  await page.goto("/organization/live-events");
  await expect(page.getByRole("heading", {name: "Групповые мероприятия"})).toBeVisible();
  await expect(page.getByText(/Публикация требует подтверждённого SFU-провайдера/)).toBeVisible();
  await expect(page.getByRole("button", {name: "Создать черновик"})).toBeDisabled();
});
