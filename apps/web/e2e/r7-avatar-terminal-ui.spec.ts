import {expect,test} from "@playwright/test";

test("R7 confirmed END returns to canonical avatar start and clears private text",async({page})=>{
 let starts=0;
 let ends=0;
 await page.route("**/v1/personas",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   available:true,personas:[{id:"persona-a",label:"Собеседник",language:"ru-RU",
    supports_text:true,supports_voice:false,supports_video:false}]
  })
 }));
 await page.route("**/v1/avatar-sessions/current",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({session:null})
 }));
 await page.route("**/v1/avatar-sessions",async route=>{
  starts++;
  await route.fulfill({status:201,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-"+starts,persona_id:"persona-a",language:"ru-RU",
    state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"},
   transport:{launch_url:"",mode:"TEXT",expires_at:""}
  })});
 });
 await page.route("**/v1/avatar-sessions/session-*/turns",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   source:"MODEL_GENERATED",reply:"Ответ собеседника"
  })
 }));
 await page.route("**/v1/avatar-sessions/session-*/actions",async route=>{
  const input=route.request().postDataJSON() as {action:string};
  expect(input.action).toBe("END");
  ends++;
  await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-"+starts,persona_id:"persona-a",language:"ru-RU",
    state:"ENDED",mode:"TEXT",safety_state:"NORMAL"},provider_close:"CONFIRMED"
  })});
 });
 await page.goto("/avatar");
 await page.getByRole("button",{name:"Начать разговор"}).click();
 await expect(page.getByRole("button",{name:"Завершить и очистить разговор"})).toBeVisible();
 await page.getByLabel("Ваше сообщение").fill("Личная фраза");
 await page.getByRole("button",{name:"Отправить ИИ"}).click();
 await expect(page.getByRole("log",{name:"Временная переписка"})).toContainText("Ответ собеседника");
 await page.getByRole("button",{name:"Завершить и очистить разговор"}).click();
 await expect(page.getByRole("button",{name:"Начать разговор"})).toBeEnabled();
 await expect(page.getByRole("log",{name:"Временная переписка"})).toHaveCount(0);
 await expect(page.getByText("Личная фраза")).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Завершить и очистить разговор"})).toHaveCount(0);
 await page.getByRole("button",{name:"Начать разговор"}).click();
 await expect(page.getByRole("button",{name:"Завершить и очистить разговор"})).toBeVisible();
 expect(starts).toBe(2);
 expect(ends).toBe(1);
});

test("R7 terminal handoff cannot repeat privileged session actions",async({page})=>{
 await page.route("**/v1/personas",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   available:true,personas:[{id:"persona-a",label:"Собеседник",language:"ru-RU",
    supports_text:true,supports_voice:false,supports_video:false}]
  })
 }));
 await page.route("**/v1/avatar-sessions/current",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-1",persona_id:"persona-a",language:"ru-RU",
    state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"}
  })
 }));
 await page.route("**/v1/avatar-sessions/session-1/actions",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-1",persona_id:"persona-a",language:"ru-RU",
    state:"ENDED",mode:"TEXT",safety_state:"NORMAL"},provider_close:"CONFIRMED"
  })
 }));
 await page.goto("/avatar");
 await page.getByRole("button",{name:"Перейти к человеку и отключить ИИ"}).click();
 await expect(page.getByText("Аватар отключён.")).toBeVisible();
 await expect(page.getByRole("button",{name:"Завершить и очистить разговор"})).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Перейти к человеку и отключить ИИ"})).toHaveCount(0);
 await expect(page.getByRole("link",{name:"Подобрать живого специалиста независимо от ИИ"})).toHaveAttribute("href","/#start");
});


test("R7 uncertain provider closure does not silently issue a fresh session",async({page})=>{
 await page.route("**/v1/personas",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   available:true,personas:[{id:"persona-a",label:"Собеседник",language:"ru-RU",
    supports_text:true,supports_voice:false,supports_video:false}]
  })
 }));
 await page.route("**/v1/avatar-sessions/current",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-1",persona_id:"persona-a",language:"ru-RU",
    state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"}
  })
 }));
 await page.route("**/v1/avatar-sessions/session-1/actions",async route=>route.fulfill({
  status:200,contentType:"application/json",body:JSON.stringify({
   session:{session_id:"session-1",persona_id:"persona-a",language:"ru-RU",
    state:"ENDED",mode:"TEXT",safety_state:"NORMAL"},provider_close:"DELIVERY_UNCERTAIN"
  })
 }));
 await page.goto("/avatar");
 await page.getByRole("button",{name:"Завершить и очистить разговор"}).click();
 await expect(page.getByText("Локальная сессия заблокирована; внешнее удаление ещё не подтверждено.")).toBeVisible();
 await expect(page.getByRole("button",{name:"Начать разговор"})).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Завершить и очистить разговор"})).toHaveCount(0);
});
