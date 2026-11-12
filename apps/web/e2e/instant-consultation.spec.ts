import { expect, test } from "@playwright/test";

test("Duty/Now requires a confirmed request and cannot create unpaid consultation access", async ({ page }) => {
  await page.goto("/now");
  await expect(page.getByRole("heading", { name: "Поговорить со специалистом сейчас" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Найти дежурного" })).toHaveCount(0);

  await page.getByLabel("С чем хотите обратиться?").fill("Мне тревожно, хочу поговорить о сне");
  await page.getByRole("button", { name: "Продолжить" }).click();
  await expect(page.getByRole("heading", { name: "Проверьте направление" })).toBeVisible();
  await page.getByLabel("Тема запроса").selectOption("sleep");
  await page.getByRole("button", { name: "Подтвердить тему" }).click();
  await expect(page.getByRole("heading", { name: "Запрос подтверждён" })).toBeVisible();

  await page.getByRole("button", { name: "Найти дежурного" }).click();
  // The isolated CI backend has no real payments or qualified duty supply.
  // A client cannot be offered a fake consultation.
  await expect(page.locator('section[aria-label="Быстрая консультация"]').getByRole("alert")).toContainText("Консультации сейчас пока недоступны");
  await expect(page.getByText("Специалист принял запрос")).toHaveCount(0);
});

test("Specialist Duty/Now is visible but does not claim unverified availability", async ({ page }) => {
  await page.goto("/specialist/now");
  await expect(page.getByRole("heading", { name: "Принимать сейчас" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Принимать сейчас/ })).toBeDisabled();
});

test("R6 home entry leads to the governed Duty/Now journey without granting access",async({page})=>{
  await page.goto("/");
  await expect(page.getByRole("navigation",{name:"Разделы"}).getByRole("link",{name:"Поговорить сейчас"})).toHaveAttribute("href","/now");
  await expect(page.getByRole("navigation",{name:"Разделы"}).getByRole("link",{name:"Обучение"})).toHaveAttribute("href","/academy");
  await expect(page.getByRole("navigation",{name:"Разделы"}).getByRole("link",{name:"Афиша"})).toHaveAttribute("href","/live-events");
  await page.locator(".hero-actions").getByRole("link",{name:"Поговорить сейчас"}).click();
  await expect(page).toHaveURL(/\/now$/);
  await expect(page.getByRole("heading",{name:"Поговорить со специалистом сейчас"})).toBeVisible();
  await expect(page.getByRole("button",{name:"Найти дежурного"})).toHaveCount(0);
});
test("R8 and R9 public home links lead to honest catalogue surfaces",async({page})=>{
  await page.goto("/");
  await page.getByRole("navigation",{name:"Разделы"}).getByRole("link",{name:"Обучение"}).click();
  await expect(page.getByRole("heading",{name:"Обучение и курсы"})).toBeVisible();
  await page.goto("/");
  await page.getByRole("navigation",{name:"Разделы"}).getByRole("link",{name:"Афиша"}).click();
  await expect(page.getByRole("heading",{name:"Встречи, вебинары и мастер-классы"})).toBeVisible();
});
