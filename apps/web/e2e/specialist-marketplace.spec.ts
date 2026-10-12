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

test("specialist page shows only real API-backed offers and unheld slots",async({page})=>{
 const p={id:"00000000-0000-0000-0000-000000000011",display_name:"Мария Смирнова",
  profession_code:"PSYCHOLOGIST",headline:"",about:"",experience_years:1,
  published_topics:["anxiety"],languages:["ru"],limitations:[]};
 await page.route("**/v1/specialists/00000000-0000-0000-0000-000000000011",async r=>r.fulfill({json:p}));
 await page.route("**/v1/specialists/00000000-0000-0000-0000-000000000011/offers",async r=>r.fulfill({
  json:{specialist_id:p.id,availability_rule:"CURRENT_ELIGIBLE_UNHELD_SLOTS",offers:[{
   id:"offer-live",topic_id:"anxiety",price_minor:230000,currency:"RUB",
   slots:[{slot_id:"slot-live",starts_at:"2026-12-14T11:00:00Z",ends_at:"2026-12-14T12:00:00Z"}]
  }]}
 }));
 await page.goto("/specialist/"+p.id);
 await expect(page.getByRole("heading",{name:"Что можно получить у этого специалиста"})).toBeVisible();
 await expect(page.getByText(/2.*300,00.*₽/)).toBeVisible();
 await expect(page.getByText(/Свободное время не резервируется при просмотре/)).toBeVisible();
 await expect(page.getByRole("link",{name:"Уточнить запрос и записаться"})).toHaveAttribute("href",`/?specialist_id=${p.id}#start`);
 await expect(page.getByRole("button",{name:"Забронировать"})).toHaveCount(0);
});
