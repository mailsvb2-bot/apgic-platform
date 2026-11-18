import {expect,test} from "@playwright/test";

// A disabled instant-duty deployment must not advertise a functioning dispatch.
// The canonical API remains the only availability owner; the UI never invents a specialist.
test("disabled instant duty is explained and cannot dispatch a confirmed request",async({page})=>{
 let dispatchAttempts=0;
 await page.route("**/v1/instant/active",route=>route.fulfill({
  status:503,contentType:"application/json",
  body:JSON.stringify({code:"DUTY_NOT_ENABLED",message_safe:"Консультации сейчас пока недоступны."})
 }));
 await page.route("**/v1/help-intents",route=>route.fulfill({
  status:201,contentType:"application/json",
  body:JSON.stringify({id:"intent-1",topics:["sleep"],goals:[],context:{}})
 }));
 await page.route("**/v1/help-intents/intent-1/confirm",route=>route.fulfill({
  status:200,contentType:"application/json",
  body:JSON.stringify({id:"intent-1",topics:["sleep"],goals:[],context:{}})
 }));
 await page.route("**/v1/instant/dispatches",route=>{
  dispatchAttempts++;
  return route.fulfill({status:500,body:"Dispatch must remain inaccessible"});
 });
 await page.goto("/now");
 const workspace=page.getByRole("region",{name:"Быстрая консультация"});
 await expect(workspace.getByRole("status").filter({hasText:"пока не подключены"})).toBeVisible();
 await expect(workspace.getByRole("link",{name:"каталог специалистов"})).toHaveAttribute("href","/specialists");
 await workspace.getByLabel("С чем хотите обратиться?").fill("Не могу уснуть, хочу поговорить со специалистом.");
 await workspace.getByRole("button",{name:"Продолжить"}).click();
 await workspace.getByRole("button",{name:"Подтвердить тему"}).click();
 await expect(workspace.getByRole("button",{name:"Найти дежурного"})).toBeDisabled();
 expect(dispatchAttempts).toBe(0);
});
