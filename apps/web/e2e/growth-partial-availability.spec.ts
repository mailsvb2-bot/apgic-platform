import {expect,test} from "@playwright/test";

// Growth enablement is distinct from the existing specialist Offer owner.
// A disabled Growth subsystem must never hide successfully read canonical offers.
test("growth unavailable keeps offers visible but blocks link creation",async({page})=>{
 let issued=0;
 await page.route("**/v1/specialist/offers",route=>route.fulfill({
  status:200,contentType:"application/json",
  body:JSON.stringify({offers:[{id:"offer-1",topic_id:"sleep",state:"ACTIVE",price_minor:150000,currency:"RUB"}]})
 }));
 await page.route("**/v1/specialist/growth-links",route=>{
  if(route.request().method()==="POST")issued++;
  return route.fulfill({status:503,contentType:"application/json",
   body:JSON.stringify({code:"GROWTH_NOT_CONFIGURED",message_safe:"Привлечение по ссылкам ещё не настроено."})});
 });
 await page.route("**/v1/specialist/growth-report",route=>route.fulfill({
  status:503,contentType:"application/json",
  body:JSON.stringify({code:"GROWTH_NOT_CONFIGURED",message_safe:"Привлечение по ссылкам ещё не настроено."})
 }));
 await page.goto("/specialist/growth");
 await expect(page.getByRole("status").filter({hasText:"пока не настроено"})).toBeVisible();
 await expect(page.getByRole("heading",{name:"sleep"})).toBeVisible();
 await expect(page.getByRole("button",{name:"Получить ссылку для предложения"})).toBeDisabled();
 expect(issued).toBe(0);
});
