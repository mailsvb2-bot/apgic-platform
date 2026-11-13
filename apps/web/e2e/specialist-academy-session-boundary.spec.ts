import {expect,test} from "@playwright/test";

// Browser-level session switch proof. These are mocked Academy responses;
// no real identity provider, PSP or production release is implied.
test("specialist Academy clears historical data when session expires and does not re-show it",async({page})=>{
 let authorized=true;
 let changedIdentity=false;
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  if(!authorized){
   await route.fulfill({status:401,contentType:"application/json",body:'{"code":"UNAUTHORIZED"}'});
   return;
  }
  const title=changedIdentity?"Новый курс другой сессии":"Старый курс прежней сессии";
  const value=path==="/v1/academy/my-courses"
   ?{courses:[{id:"course-1",title,state:"PUBLISHED",version_no:1}]}
   :{courses:[{course_id:"course-1",title,enrollments:3,completed:2,pending_assessments:1,passed_assessments:2}],
     gross_captures:[{course_id:"course-1",currency:"RUB",captured_count:1,
       captured_gross_minor:changedIdentity?200:123456,active_count:1,active_gross_minor:200}],
     revenue_state:"GROSS_CAPTURED_ONLY"};
  await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify(value)});
 });
 await page.goto("/specialist");
 const workspace=page.locator("#specialist-academy");
 await expect(workspace.getByText("Старый курс прежней сессии").first()).toBeVisible();
 await expect(workspace.getByText(/123456 RUB/)).toBeVisible();

 authorized=false;
 await workspace.getByRole("button",{name:"Обновить данные"}).click();
 await expect(workspace.getByRole("status")).toContainText("Войдите в свой аккаунт APGIC");
 await expect(workspace.getByText("Старый курс прежней сессии")).toHaveCount(0);
 await expect(workspace.getByText(/123456 RUB/)).toHaveCount(0);
 await expect(workspace.getByText("Зачислено: —")).toBeVisible();
 await expect(workspace.getByText("Ожидают проверки: —")).toBeVisible();

 authorized=true;
 changedIdentity=true;
 await workspace.getByRole("button",{name:"Обновить данные"}).click();
 await expect(workspace.getByText("Новый курс другой сессии").first()).toBeVisible();
 await expect(workspace.getByText("Старый курс прежней сессии")).toHaveCount(0);
 await expect(workspace.getByText(/123456 RUB/)).toHaveCount(0);
});
