import {expect, test, type Page} from "@playwright/test";

// Browser integration with a simulated canonical Academy response. This is
// deliberately NOT evidence of a live PSP, Yandex OAuth or staging journey.
const enrollmentID="00000000-0000-4000-8000-000000000801";
const lessonID="00000000-0000-4000-8000-000000000802";
const mediaID="00000000-0000-4000-8000-000000000803";

async function mockAcademyVideo(page:Page){
 let allowed=true;
 let manifestReads=0;
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  let data:unknown={};
  let status=200;
  if(path==="/v1/academy/courses"||path==="/v1/academy/my-courses")data={courses:[]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[{
   id:enrollmentID,course_title:"Проверочный курс",version_no:1,
   state:"ACTIVE",access_state:"ACTIVE"
  }]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts"||path.endsWith("/cohorts"))data={cohorts:[]};
  else if(path==="/v1/academy/review/attempts"||path.endsWith("/attempts"))data={attempts:[]};
  else if(path==="/v1/academy/me/appeals")data={appeals:[]};
  else if(path.endsWith("/learn"))data={lessons:[{
   id:lessonID,title:"Защищённый видеоурок",kind:"VIDEO",ordinal:1,available:true
  }]};
  else if(path.endsWith("/videos")){
   manifestReads++;
   if(allowed)data={videos:[{id:mediaID,title:"Видео автора",ordinal:1}]};
   else {status=403;data={code:"ENTITLEMENT_REVOKED"}}
  }else if(path.endsWith("/stream")){
   // No fake media is needed: tests dispatch HTMLMediaElement events.
   status=403;data={code:"TEST_NO_MEDIA"};
  }
  else {status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path}}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Обучение и курсы"})).toBeVisible();
 await expect(page.getByText("Проверочный курс")).toBeVisible();
 await page.getByRole("button",{name:"Продолжить"}).click();
 await expect(page.locator("video")).toHaveCount(1);
 return {revoke:()=>{allowed=false},reads:()=>manifestReads};
}

test("HTML5 media errors discard buffered source and retry requires canonical access",async({page})=>{
 const access=await mockAcademyVideo(page);
 const video=page.locator("video");
 await expect(video).toHaveAttribute("src",/\/stream$/);
 await video.evaluate(element=>element.dispatchEvent(new Event("error")));
 await expect(video).not.toHaveAttribute("src",/\/stream$/);
 await expect(page.getByRole("button",{name:"Повторить загрузку видео"})).toBeVisible();
 access.revoke();
 await page.getByRole("button",{name:"Повторить загрузку видео"}).click();
 await expect(video).not.toHaveAttribute("src",/\/stream$/);
 await expect(page.getByRole("alert").filter({hasText:"Не удалось подтвердить доступ или возобновить просмотр"})).toBeVisible();
 expect(access.reads()).toBeGreaterThanOrEqual(2);
});

test("revoked canonical manifest stops an opened player and hides its video",async({page})=>{
 const access=await mockAcademyVideo(page);
 await page.locator("video").evaluate(element=>element.dispatchEvent(new Event("play")));
 await expect.poll(access.reads).toBeGreaterThanOrEqual(2);
 access.revoke();
 await expect(page.locator("video")).toHaveCount(0);
 await expect(page.getByText(/Доступ к видео не подтверждён или был отозван/)).toBeVisible();
});

// A browser can suspend timers in background tabs and keep decoded media in
// memory (including bfcache). Returning to the page must never resume the
// previous stream without a fresh canonical Entitlement read.
test("pagehide discards buffered video and recovery requires canonical access",async({page})=>{
 const access=await mockAcademyVideo(page);
 await expect(page.locator("video")).toHaveAttribute("src",/\\/stream$/);
 await page.evaluate(()=>window.dispatchEvent(new PageTransitionEvent("pagehide",{persisted:true})));
 await expect(page.locator("video")).toHaveCount(0);
 await expect(page.getByText(/Просмотр приостановлен/)).toBeVisible();
 access.revoke();
 await page.getByRole("button",{name:"Проверить доступ повторно"}).click();
 await expect(page.locator("video")).toHaveCount(0);
 await expect(page.getByText(/Не удалось получить список видео/)).toBeVisible();
 expect(access.reads()).toBeGreaterThanOrEqual(2);
});

test("hidden tab unloads HTML5 video instead of relying on throttled polling",async({page})=>{
 await mockAcademyVideo(page);
 await expect(page.locator("video")).toHaveAttribute("src",/\\/stream$/);
 await page.evaluate(()=>{
  const original=Object.getOwnPropertyDescriptor(document,"visibilityState");
  Object.defineProperty(document,"visibilityState",{configurable:true,get:()=>"hidden"});
  try{document.dispatchEvent(new Event("visibilitychange"))}
  finally{
   if(original)Object.defineProperty(document,"visibilityState",original);
   else Reflect.deleteProperty(document,"visibilityState");
  }
 });
 await expect(page.locator("video")).toHaveCount(0);
 await expect(page.getByText(/Просмотр приостановлен/)).toBeVisible();
});
