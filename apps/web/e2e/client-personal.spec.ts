import {expect,test} from "@playwright/test";

const booking="123e4567-e89b-42d3-a456-426614174001";
const identity="123e4567-e89b-42d3-a456-426614174000";
const makeView=(more=false)=>({
  identity_id:identity,as_of:"2026-10-12T10:00:00Z",
  preferences:{display_name:"Анна",locale:"ru",time_zone:"Europe/Moscow"},
  upcoming:[{booking_id:booking,state:"CONFIRMED",starts_at:"2026-10-15T14:00:00Z",ends_at:"2026-10-15T15:00:00Z",specialist_name:"Ирина Тестовая",specialist_profession:"PSYCHOLOGIST"}],
  history:more?[{booking_id:"123e4567-e89b-42d3-a456-426614174002",state:"COMPLETED",starts_at:"2026-10-01T12:00:00Z",ends_at:"2026-10-01T13:00:00Z",specialist_name:"Ирина Тестовая",specialist_profession:"PSYCHOLOGIST",consultation_state:"COMPLETED"}]:[],
  requests:[{id:"123e4567-e89b-42d3-a456-426614174003",state:"CONFIRMED",topics:["sleep"],created_at:"2026-10-09T12:00:00Z"}],
  ...(!more?{next_history_cursor:"test-cursor"}:{}),
});
test("personal workspace loads owner snapshot, saves preferences and advances history",async({page})=>{
  let saved=false;
  await page.route("**/v1/client/personal/preferences",route=>{
    saved=true;
    return route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({display_name:"Аня",locale:"ru",time_zone:"Europe/Moscow"})});
  });
  await page.route("**/v1/client/personal**",route=>{
    if(new URL(route.request().url()).pathname!=="/v1/client/personal")return route.fallback();
    const more=new URL(route.request().url()).searchParams.has("history_cursor");
    return route.fulfill({status:200,contentType:"application/json",headers:{"cache-control":"no-store"},body:JSON.stringify(makeView(more))});
  });
  await page.goto("/personal");
  await expect(page.getByRole("heading",{name:"Мой кабинет"})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("button",{name:"Показать более ранние записи"}).click();
  await expect(page.getByText("Завершена").first()).toBeVisible();
  await page.getByRole("textbox",{name:"Как к вам обращаться"}).fill("Аня");
  await page.getByRole("button",{name:"Сохранить настройки"}).click();
  await expect.poll(()=>saved).toBe(true);
});
test("personal workspace fails closed without signed session",async({page})=>{
  await page.route("**/v1/client/personal",r=>r.fulfill({status:401,body:'{"code":"CLIENT_SESSION_REQUIRED"}'}));
  await page.goto("/personal");
  await expect(page.getByRole("alert")).toContainText("Нет подтверждённой сессии");
  await expect(page.getByRole("heading",{name:"Предстоящие записи"})).toHaveCount(0);
});
