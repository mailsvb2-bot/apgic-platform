import {expect,test} from "@playwright/test";

// Non-mutating browser smoke coverage of the public routes and the anonymous
// access boundaries. Authenticated actions are covered by their dedicated E2E
// suites; this test must never write to a real account or payment provider.
const publicRoutes=[
 "/", "/personal", "/account", "/organization", "/specialist", "/academy",
 "/specialists", "/live-events", "/now", "/avatar", "/specialist/now",
 "/specialist/growth", "/finance/operations", "/organization/live-events",
 "/update", "/help/trevoga", "/help/son", "/help/otnosheniya", "/help/kariera"
];

test("all published entry surfaces render, have named controls and do not overflow",async({page})=>{
 test.setTimeout(120_000);
 for(const path of publicRoutes){
  const runtimeErrors:string[]=[];
  const onError=(error:Error)=>runtimeErrors.push(error.message);
  page.on("pageerror",onError);
  try{
   const response=await page.goto(path,{waitUntil:"domcontentloaded"});
   expect(response?.status(),`route ${path} must respond`).toBe(200);
   await expect(page.locator("body")).not.toBeEmpty();
   const ui=await page.evaluate(()=>{
    const visible=(node:Element)=>{const bounds=node.getBoundingClientRect();return bounds.width>0&&bounds.height>0};
    const unnamedButtons=[...document.querySelectorAll("button")].filter(node=>
     visible(node)&&!((node.textContent||"").trim()||node.getAttribute("aria-label")||node.getAttribute("title")));
    const brokenAnchors=[...document.querySelectorAll('a[href^="#"]')].map(node=>node.getAttribute("href")||"")
     .filter(href=>href.length>1&&!document.getElementById(href.substring(1)));
    return {overflow:document.documentElement.scrollWidth-innerWidth,
     unnamedButtons:unnamedButtons.length,brokenAnchors,
     hasMain:!!document.querySelector("main")};
   });
   expect(ui.hasMain,`main landmark on ${path}`).toBe(true);
   expect(ui.overflow,`horizontal overflow on ${path}`).toBeLessThanOrEqual(2);
   expect(ui.unnamedButtons,`unnamed visible buttons on ${path}`).toBe(0);
   expect(ui.brokenAnchors,`dangling anchors on ${path}`).toEqual([]);
   expect(runtimeErrors,`JavaScript failures on ${path}`).toEqual([]);
  }finally{page.off("pageerror",onError)}
 }
});

test("specialist and Academy navigation points to working canonical workspaces",async({page})=>{
 await page.goto("/specialist");
 const nav=page.getByRole("navigation",{name:"Навигация кабинета специалиста"});
 for(const label of ["Профиль","О себе","Направления","Документы","Публикация","Услуги и цены","Расписание","Консультации","Курсы и ученики"]){
  await nav.getByRole("button",{name:label}).click();
  await expect(nav.getByRole("button",{name:label})).toHaveAttribute("aria-current","page");
 }
 await page.goto("/academy");
 const academy=page.getByRole("navigation",{name:"Разделы кабинета"});
 for(const label of ["Мои курсы","Моё обучение","Каталог","Проверка работ","Ученики и продажи"]){
  await academy.getByRole("button",{name:label}).click();
  await expect(academy.getByRole("button",{name:label})).toHaveAttribute("aria-current","page");
 }
});
