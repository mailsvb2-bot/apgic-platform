import {expect, test, type Page} from "@playwright/test";
import {readFileSync} from "node:fs";
import {join} from "node:path";

// Browser integration with a simulated canonical Academy response. This is
// deliberately NOT evidence of a live PSP, Yandex OAuth or staging journey.
const enrollmentID="00000000-0000-4000-8000-000000000801";
const lessonID="00000000-0000-4000-8000-000000000802";
const mediaID="00000000-0000-4000-8000-000000000803";
// A tiny actual MP4 keeps Chromium from spontaneously firing media errors
// before tests can exercise revocation. This is NOT live Yandex media evidence.
const mediaBytes=Buffer.from(readFileSync(join(process.cwd(),"e2e","fixtures","academy-video.mp4.base64"),"utf8").trim(),"base64");

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
   // The mocked media gateway must enforce the same revoked-access boundary
   // as the manifest; otherwise the browser tests mask a direct-byte leak.
   if(!allowed){
    await route.fulfill({status:403,contentType:"application/json",body:JSON.stringify({code:"ENTITLEMENT_REVOKED"})});
    return;
   }
   // Serve real decodable fixture bytes through a mocked Academy endpoint.
   // Preserve HTTP Range semantics so HTML5 metadata/seek requests succeed.
   const range=route.request().headers()["range"];
   const match=range?.match(/^bytes=(\d+)-(\d*)$/);
   if(range&&!match){
    await route.fulfill({status:416,headers:{"Content-Range":`bytes */${mediaBytes.length}`},body:""});
    return;
   }
   const start=match?Number(match[1]):0;
   const end=match&&match[2]?Math.min(Number(match[2]),mediaBytes.length-1):mediaBytes.length-1;
   if(start>=mediaBytes.length||start>end){
    await route.fulfill({status:416,headers:{"Content-Range":`bytes */${mediaBytes.length}`},body:""});
    return;
   }
   const partial=Boolean(match);
   const body=mediaBytes.subarray(start,end+1);
   await route.fulfill({status:partial?206:200,headers:{
    "Content-Type":"video/mp4","Accept-Ranges":"bytes",
    "Content-Length":String(body.length),
    ...(partial?{"Content-Range":`bytes ${start}-${end}/${mediaBytes.length}`}:{})
   },body});
   return;
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

// Revocation must be observed even when the browser has attached the media
// source but has not emitted play, loadedmetadata or progress. A preload=none
// element can still hold a provider response, so event-triggered polling alone
// leaves a revocation window before the first user interaction.
test("revocation before any player event unloads an attached video source",async({page})=>{
 const access=await mockAcademyVideo(page);
 const video=page.locator("video");
 await expect(video).toHaveAttribute("src",/\/stream$/);
 await expect.poll(access.reads,{timeout:8000}).toBeGreaterThanOrEqual(2);
 access.revoke();
 await expect(video).toHaveCount(0);
 await expect(page.getByText(/Доступ к видео не подтверждён или был отозван/)).toBeVisible();
});

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

// A retry button awaits the canonical manifest before reattaching the source.
// Browser user activation can expire during that fetch, so retry must not
// invoke play() automatically and mistake an autoplay policy denial for a
// revoked Entitlement or a broken media gateway.
test("authorized retry restores HTML5 source without asynchronous autoplay",async({page})=>{
 await mockAcademyVideo(page);
 const video=page.locator("video");
 await expect(video).toHaveAttribute("src",/\/stream$/);
 await video.evaluate(element=>{
  const player=element as HTMLVideoElement;
  Object.defineProperty(player,"play",{
   configurable:true,
   value:()=>Promise.reject(new DOMException("Autoplay requires activation","NotAllowedError"))
  });
  player.dispatchEvent(new Event("error"));
 });
 await expect(video).not.toHaveAttribute("src",/\/stream$/);
 await page.getByRole("button",{name:"Повторить загрузку видео"}).click();
 await expect(video).toHaveAttribute("src",/\/stream$/);
 await expect(page.getByRole("button",{name:"Повторить загрузку видео"})).toHaveCount(0);
});

test("revoked canonical manifest stops an opened player and hides its video",async({page})=>{
 const access=await mockAcademyVideo(page);
 await page.locator("video").evaluate(element=>element.dispatchEvent(new Event("play")));
 await expect.poll(access.reads).toBeGreaterThanOrEqual(2);
 access.revoke();
 await expect(page.locator("video")).toHaveCount(0);
 await expect(page.getByText(/Доступ к видео не подтверждён или был отозван/)).toBeVisible();
});

// A browser may populate metadata or media buffers before the first play
// event. Polling must start on those events, including while video is paused.
for(const eventName of ["loadedmetadata","progress"] as const){
 test(`revocation clears buffered video after ${eventName} without play`,async({page})=>{
  const access=await mockAcademyVideo(page);
  const video=page.locator("video");
  await expect(video).toHaveAttribute("src",/\/stream$/);
  await video.evaluate((element,event)=>element.dispatchEvent(new Event(event)),eventName);
  await expect.poll(access.reads,{timeout:8000}).toBeGreaterThanOrEqual(2);
  access.revoke();
  await expect(video).toHaveCount(0);
  await expect(page.getByText(/Доступ к видео не подтверждён или был отозван/)).toBeVisible();
 });
}

// Repeated successful manifest checks must keep the player open, and a
// subsequent canonical denial must still remove the source. This exercises
// polling across several intervals rather than only the initial read.
test("video stays authorized across repeated polls, then revocation unloads it",async({page})=>{
 const access=await mockAcademyVideo(page);
 const video=page.locator("video");
 await video.evaluate(element=>element.dispatchEvent(new Event("play")));
 await expect.poll(access.reads,{timeout:8000}).toBeGreaterThanOrEqual(4);
 await expect(video).toHaveAttribute("src",/\/stream$/);
 access.revoke();
 await expect(video).toHaveCount(0);
 await expect(page.getByText(/Доступ к видео не подтверждён или был отозван/)).toBeVisible();
});

// A browser can suspend timers in background tabs and keep decoded media in
// memory (including bfcache). Returning to the page must never resume the
// previous stream without a fresh canonical Entitlement read.
test("pagehide discards buffered video and recovery requires canonical access",async({page})=>{
 const access=await mockAcademyVideo(page);
 await expect(page.locator("video")).toHaveAttribute("src",/\/stream$/);
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
 await expect(page.locator("video")).toHaveAttribute("src",/\/stream$/);
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
