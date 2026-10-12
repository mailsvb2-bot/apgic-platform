import {expect,test} from "@playwright/test";

const profile={
 id:"00000000-0000-0000-0000-000000000011",display_name:"Мария Смирнова",
 profession_code:"PSYCHOLOGIST",headline:"Психолог, работа с тревожностью",
 about:"Опора на навыки саморегуляции",experience_years:8,
 languages:["ru"],limitations:["Не работаю с детьми"],published_topics:["anxiety"],
};

test("public marketplace supports filter, reviewed profile view and route back to confirmed selection",async({page})=>{
 await page.route("**/v1/specialists?*",async route=>{
  const url=new URL(route.request().url());
  const topic=url.searchParams.get("topic");
  await route.fulfill({json:{
   profiles:topic==="sleep"?[]:[profile],has_more:false,limit:10,offset:0,
   topic:topic??"",policy_version:"qualification-v1",sort:"DISPLAY_NAME_ASC"
  }});
 });
 await page.route("**/v1/specialists/00000000-0000-0000-0000-000000000011",async route=>{
  await route.fulfill({json:profile});
 });
 await page.goto("/specialists");
 await expect(page.getByRole("heading",{name:"Выберите специалиста"})).toBeVisible();
 await expect(page.getByRole("heading",{name:"Мария Смирнова"})).toBeVisible();
 await page.getByLabel("Направление").selectOption("sleep");
 await expect(page.getByText("Сейчас нет опубликованных проверенных анкет")).toBeVisible();
 await page.getByLabel("Направление").selectOption("anxiety");
 await page.getByRole("link",{name:"Посмотреть анкету"}).click();
 await expect(page.getByRole("heading",{name:"Мария Смирнова",level:1})).toBeVisible();
 await expect(page.getByText("Опора на навыки саморегуляции")).toBeVisible();
 await expect(page.getByText("Не работаю с детьми")).toBeVisible();
 const continueLink=page.getByRole("link",{name:"Уточнить запрос и выбрать время"});
 await expect(continueLink).toHaveAttribute("href","/?specialist_id=00000000-0000-0000-0000-000000000011#start");
 await expect(page.getByText("evidence_ref")).toHaveCount(0);
});

test("revoked publication cannot be viewed even with a bookmarked direct URL",async({page})=>{
 await page.route("**/v1/specialists/00000000-0000-0000-0000-000000000011",async route=>{
  await route.fulfill({status:404,json:{code:"SPECIALIST_NOT_PUBLIC",message_safe:"Not public"}});
 });
 await page.goto("/specialist/00000000-0000-0000-0000-000000000011");
 await expect(page.getByRole("heading",{name:"Анкета не опубликована"})).toBeVisible();
 await expect(page.getByText("Опора на навыки саморегуляции")).toHaveCount(0);
});
