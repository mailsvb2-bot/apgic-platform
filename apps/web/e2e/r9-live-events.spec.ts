import {expect, test} from "@playwright/test";

test("R9 public event route remains truthful when feature gate is closed", async ({page}) => {
  await page.goto("/live-events");
  await expect(page.getByRole("heading", {name: "Встречи, вебинары и мастер-классы"})).toBeVisible();
  await expect(page.locator(".live-events-panel [role=alert]")).toContainText("Раздел пока не подключён.");
  await expect(page.getByText("Сейчас нет опубликованных мероприятий.")).toHaveCount(0);
});

test("R9 organizer route is discoverable without inventing media readiness", async ({page}) => {
  await page.goto("/organization/live-events");
  await expect(page.getByRole("heading", {name: "Групповые мероприятия"})).toBeVisible();
  await expect(page.getByText(/Публикация требует подтверждённого SFU-провайдера/)).toBeVisible();
  await expect(page.getByRole("button", {name: "Создать черновик"})).toBeDisabled();
});


test("R9 registration outage never disguises a registered visitor as unregistered",async({page})=>{
 let registrationStatus=503;
 await page.route("**/v1/live-events/event-1",async route=>{
  await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
   id:"event-1",title:"Практикум",description:"Групповое мероприятие",format:"WORKSHOP",
   starts_at:"2026-12-01T12:00:00Z",ends_at:"2026-12-01T13:00:00Z",
   capacity:10,registered:2,state:"PUBLISHED",price_minor:0,recording_available:false
  })});
 });
 await page.route("**/v1/live-events/event-1/registration",async route=>{
  await route.fulfill({status:registrationStatus,contentType:"application/json",body:JSON.stringify({code:"UNAVAILABLE"})});
 });
 await page.goto("/live-events/event-1");
 await expect(page.getByRole("heading",{name:"Практикум"})).toBeVisible();
 await expect(page.getByText("Статус участия не подтверждён.")).toBeVisible();
 await expect(page.getByRole("button",{name:"Зарегистрироваться бесплатно"})).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Получить защищённый вход в эфир"})).toHaveCount(0);
 registrationStatus=404;
 await page.getByRole("button",{name:"Повторить проверку регистрации"}).click();
 await expect(page.getByRole("button",{name:"Зарегистрироваться бесплатно"})).toBeEnabled();
 await expect(page.getByText("Статус участия не подтверждён.")).toHaveCount(0);
});

test("R9 unauthorized registration readback cannot enable event mutations",async({page})=>{
 await page.route("**/v1/live-events/event-2",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   id:"event-2",title:"Вебинар",description:"Программа",format:"WEBINAR",
   starts_at:"2026-12-01T12:00:00Z",ends_at:"2026-12-01T13:00:00Z",
   capacity:20,registered:1,state:"PUBLISHED",price_minor:0,recording_available:false
  })
 }));
 await page.route("**/v1/live-events/event-2/registration",async route=>route.fulfill({
  status:401,contentType:"application/json",body:JSON.stringify({code:"SESSION_REQUIRED"})
 }));
 await page.goto("/live-events/event-2");
 await expect(page.getByText("Для проверки регистрации требуется действующая сессия.")).toBeVisible();
 await expect(page.getByRole("button",{name:"Зарегистрироваться бесплатно"})).toHaveCount(0);
});
