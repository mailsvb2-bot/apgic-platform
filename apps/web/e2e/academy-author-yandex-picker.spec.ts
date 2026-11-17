import {Buffer} from "node:buffer";
import {expect, test} from "@playwright/test";

// This browser test locks the author-facing sequence after a successful
// Yandex OAuth callback. Provider OAuth and Yandex's own API are deliberately
// mocked here; this is a UI regression test, not live-provider acceptance.
const courseID="11111111-1111-4111-8111-111111111111";
const versionID="22222222-2222-4222-8222-222222222222";
const moduleID="33333333-3333-4333-8333-333333333333";
const lessonID="44444444-4444-4444-8444-444444444444";
const mediaID="55555555-5555-4555-8555-555555555555";
const existingMediaID="88888888-8888-4888-8888-888888888888";
const diskFilePath="disk:/Лекции/Урок 1.mp4";
const learnerVideoFixture=Buffer.from("GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwEAAAAAAAJHEU2bdLpNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHWTbuMU6uEElTDZ1OsggEjTbuMU6uEHFO7a1OsggIx7AEAAAAAAABZAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmsCrXsYMPQkBNgIxMYXZmNjEuNy4xMDNXQYxMYXZmNjEuNy4xMDNEiYhAn0AAAAAAABZUrmvIrgEAAAAAAAA/14EBc8WIXe2KIQMHVf6cgQAitZyDdW5kiIEAhoVWX1ZQOIOBASPjg4QdzWUA4JCwgVC6gS2agQJVsIRVuYEBElTDZ/tzc59jwIBnyJlFo4dFTkNPREVSRIeMTGF2ZjYxLjcuMTAzc3PWY8CLY8WIXe2KIQMHVf5nyKFFo4dFTkNPREVSRIeUTGF2YzYxLjE5LjEwMSBsaWJ2cHhnyKFFo4hEVVJBVElPTkSHkzAwOjAwOjAyLjAwMDAwMDAwMAAfQ7Z1QIjngQCjqYEAAIDQAgCdASpQAC0AAMcIhYWImYSIBAIABnZBXTHWACrjwP7/DWQAo5yBAfQAEQIAAxAoABgAGe/0DAABQFbCkP7+zmAAo5yBA+gAEQIAAxAcABgAGe/0DAABQFbCkP7/YBgAo5yBBdwAEQIAAxAQABgAGe/0DAABQFbCkP7/q1CAHFO7a5G7j7OBALeK94EB8YIBo/CBAw==","base64");

test("author resumes the selected course and lesson, imports Disk video, and saves its lesson mapping",async({page})=>{
 const course={
  id:courseID,product_id:"66666666-6666-4666-8666-666666666666",
  title:"Проверочный курс",description:"",state:"DRAFT",version_id:versionID,version_no:1
 };
 const media: {id:string;title:string;provider:string;provider_file_ref:string;active:boolean}[]=[
  {id:existingMediaID,title:"Уже добавленное видео.mp4",provider:"YANDEX_DISK",provider_file_ref:"disk:/Уже добавленное видео.mp4",active:true}
 ];
 let assignments:{lesson_id:string;media_id:string;ordinal:number}[]=[];
 let savedPayload:unknown;
 let importCalls=0;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request();
  const url=new URL(request.url());
  const path=url.pathname;
  const method=request.method();
  let status=200;
  let data:unknown={};
  if(method==="GET"&&path==="/v1/academy/courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-courses")data={courses:[course]};
  else if(method==="GET"&&path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(method==="GET"&&path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(method==="GET"&&path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(method==="GET"&&path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(method==="GET"&&path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(method==="GET"&&path==="/v1/academy/courses/"+courseID+"/versions/"+versionID+"/outline"){
   data={state:"DRAFT",modules:[{id:moduleID,version_id:versionID,title:"Модуль 1",ordinal:1}],
    lessons:[{id:lessonID,module_id:moduleID,title:"Урок 1",kind:"VIDEO",ordinal:1}]};
  }else if(method==="GET"&&path==="/v1/academy/author/cloud-media")data={media};
  else if(method==="GET"&&path==="/v1/academy/author/courses/"+courseID+"/versions/"+versionID+"/media"){
   data={assignments:assignments.map(item=>({...item,title:"Урок 1.mp4",provider:"YANDEX_DISK",provider_file_ref:diskFilePath}))};
  }else if(method==="GET"&&path==="/v1/academy/author/cloud/yandex/files"){
   const selectedPath=url.searchParams.get("path")||"disk:/";
   const files=selectedPath==="disk:/"
    ?[{name:"Лекции",path:"disk:/Лекции",type:"dir",mime_type:"",size:0}]
    :selectedPath==="disk:/Лекции"
     ?[{name:"Урок 1.mp4",path:diskFilePath,type:"file",mime_type:"video/mp4",size:1024}]
     :[];
   data={files,total:files.length,offset:Number(url.searchParams.get("offset")||0)};
  }else if(method==="POST"&&path==="/v1/academy/author/cloud-media/import"){
   const body=request.postDataJSON() as {items:{provider_file_ref:string;title:string}[]};
   importCalls++;
   const added=body.items.map(item=>({id:mediaID,title:item.title,provider:"YANDEX_DISK",provider_file_ref:item.provider_file_ref,active:true}));
   media.push(...added);
   // The canonical import endpoint returns only this request's imported
   // items, not the entire pre-existing author media library.
   status=201;data={media:added};
  }else if(method==="POST"&&path==="/v1/academy/courses/"+courseID+"/versions/"+versionID+"/media/assign"){
   const body=request.postDataJSON() as {items:{lesson_id:string;media_id:string;ordinal:number}[]};
   savedPayload=body.items;
   assignments=body.items;
   data={state:"SAVED"};
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });

 await page.goto("/academy?cloud_connected=yandex&course_id="+courseID+"&version_id="+versionID+"&lesson_id="+lessonID);
 await expect(page.getByRole("heading",{name:"Проверочный курс"})).toBeVisible();
 await expect(page.getByRole("button",{name:"2. Видео"})).toHaveAttribute("aria-current","step");
 await expect(page.getByLabel("К какому уроку добавить видео?")).toHaveValue(lessonID);
 await expect(page.locator("#academy-video-lesson optgroup")).toHaveAttribute("label","1. Модуль 1");
 await expect(page.locator('#academy-media-editor p[role="status"]').filter({hasText:"Редактируем:"})).toContainText("Модуль 1");
 await page.getByText("Добавить новые файлы с Яндекс Диска").click();
 await expect(page.getByRole("button",{name:"Папка: Лекции"})).toBeVisible();

 await page.getByRole("button",{name:"Папка: Лекции"}).click();
 await expect(page.getByRole("checkbox",{name:"Урок 1.mp4"})).toBeVisible();
 await page.getByRole("checkbox",{name:"Урок 1.mp4"}).check();
 await page.getByRole("button",{name:"Добавить к уроку (1)"}).click();
 await expect(page.getByText("Видео добавлены к выбранному уроку. Нажмите «Сохранить видео урока».")).toBeVisible();
 await expect(page.getByRole("heading",{name:"Уже добавленные видео в медиатеке"})).toBeVisible();
 expect(importCalls).toBe(1);
 // Previously imported cloud videos attach through the primary lesson action,
 // without a second Yandex import and without replacing the just added file.
 await page.getByRole("checkbox",{name:"Уже добавленное видео.mp4"}).check();
 await page.getByRole("button",{name:"Прикрепить выбранные видео (1)"}).click();
 await expect(page.getByText("Видео добавлены к уроку без удаления предыдущих.",{exact:false})).toBeVisible();
 await expect(page.getByRole("button",{name:"Выше",exact:true})).toHaveCount(2);
 expect(importCalls).toBe(1);

 await page.getByRole("button",{name:"Сохранить видео урока"}).click();
 await expect(page.getByText("Распределение сохранено")).toBeVisible();
 expect(savedPayload).toEqual([
  {lesson_id:lessonID,media_id:mediaID,ordinal:1},
  {lesson_id:lessonID,media_id:existingMediaID,ordinal:2}
 ]);
 expect(assignments).toEqual(savedPayload);
});


test("author creates a video lesson before selecting its Yandex Disk file",async({page})=>{
 const course={id:courseID,product_id:"66666666-6666-4666-8666-666666666666",
  title:"Проверочный курс",description:"",state:"DRAFT",version_id:versionID,version_no:1};
 const lessons:{id:string;module_id:string;title:string;kind:string;ordinal:number}[]=[];
 const media:{id:string;title:string;provider:string;provider_file_ref:string;active:boolean}[]=[];
 let savedAssignments:{lesson_id:string;media_id:string;ordinal:number}[]=[];
 let createdLessonBody:Record<string,unknown>|undefined;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname,method=request.method();
  let status=200,data:unknown={};
  if(method==="GET"&&path==="/v1/academy/courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-courses")data={courses:[course]};
  else if(method==="GET"&&path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(method==="GET"&&path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(method==="GET"&&path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(method==="GET"&&path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(method==="GET"&&path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(method==="GET"&&path==="/v1/academy/courses/"+courseID+"/versions/"+versionID+"/outline"){
   data={state:"DRAFT",modules:[{id:moduleID,version_id:versionID,title:"Модуль 1",ordinal:1}],lessons};
  }else if(method==="POST"&&path==="/v1/academy/courses/"+courseID+"/versions/"+versionID+"/modules/"+moduleID+"/lessons"){
   createdLessonBody=request.postDataJSON() as Record<string,unknown>;
   lessons.push({id:lessonID,module_id:moduleID,title:"Урок 1",kind:"VIDEO",ordinal:1});
   status=201;data=lessons[0];
  }else if(method==="GET"&&path==="/v1/academy/author/cloud-media")data={media};
  else if(method==="GET"&&path==="/v1/academy/author/courses/"+courseID+"/versions/"+versionID+"/media"){
   data={assignments:savedAssignments.map(item=>({...item,title:"Урок 1.mp4",provider:"YANDEX_DISK",provider_file_ref:diskFilePath}))};
  }else if(method==="GET"&&path==="/v1/academy/author/cloud/yandex/files"){
   const selectedPath=url.searchParams.get("path")||"disk:/";
   const files=selectedPath==="disk:/"
    ?[{name:"Лекции",path:"disk:/Лекции",type:"dir",mime_type:"",size:0}]
    :selectedPath==="disk:/Лекции"
     ?[{name:"Урок 1.mp4",path:diskFilePath,type:"file",mime_type:"video/mp4",size:1024}]
     :[];
   data={files,total:files.length,offset:Number(url.searchParams.get("offset")||0)};
  }else if(method==="POST"&&path==="/v1/academy/author/cloud-media/import"){
   const body=request.postDataJSON() as {items:{provider_file_ref:string;title:string}[]};
   for(const item of body.items)media.push({id:mediaID,title:item.title,provider:"YANDEX_DISK",provider_file_ref:item.provider_file_ref,active:true});
   status=201;data={media:media.slice()};
  }else if(method==="POST"&&path==="/v1/academy/courses/"+courseID+"/versions/"+versionID+"/media/assign"){
   const body=request.postDataJSON() as {items:{lesson_id:string;media_id:string;ordinal:number}[]};
   savedAssignments=body.items;data={state:"SAVED"};
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });

 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Кабинет автора"})).toBeVisible();
 await page.getByRole("button",{name:"Мои курсы"}).click();
 await expect(page.getByRole("heading",{name:"Ваши курсы"})).toBeVisible();
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await expect(page.locator("#academy-lesson-title")).toBeVisible();
 await page.getByRole("button",{name:"2. Видео"}).click();
 await expect(page.getByText("В этом курсе пока нет видеоуроков.",{exact:false})).toBeVisible();
 await page.getByRole("button",{name:"1. Уроки"}).click();
 await page.locator("#academy-lesson-title").fill("Урок 1");
 // Simulate switching from a text lesson to the VIDEO default. Hidden text
 // must not leak into the canonical video attachment field.
 await page.locator("#academy-lesson-kind").selectOption("READING");
 await page.locator("#academy-lesson-body").fill("Старый текстовый материал");
 await page.locator("#academy-lesson-kind").selectOption("VIDEO");
 await page.getByRole("button",{name:"Добавить урок",exact:true}).click();
 await expect(page.getByLabel("К какому уроку добавить видео?")).toBeVisible();
 expect(createdLessonBody?.kind).toBe("VIDEO");
 expect(createdLessonBody?.body).toBe("");
 expect(createdLessonBody?.media_asset_ref).toBeUndefined();

 // Creating a VIDEO lesson must open its own media editor automatically.
 await expect(page.getByLabel("К какому уроку добавить видео?")).toHaveValue(lessonID);
 await expect(page.getByText("Медиатека пока пуста.",{exact:false})).toBeVisible();
 await page.getByText("Добавить новые файлы с Яндекс Диска").click();
 await page.getByRole("button",{name:"Открыть Яндекс Диск"}).click();
 await page.getByRole("button",{name:"Папка: Лекции"}).click();
 await page.getByRole("checkbox",{name:"Урок 1.mp4"}).check();
 await page.getByRole("button",{name:"Добавить к уроку (1)"}).click();
 await expect(page.getByText("Видео добавлены к выбранному уроку. Нажмите «Сохранить видео урока».")).toBeVisible();
 await page.getByRole("button",{name:"Сохранить видео урока"}).click();
 await expect(page.getByText("Распределение сохранено")).toBeVisible();
 expect(savedAssignments).toEqual([{lesson_id:lessonID,media_id:mediaID,ordinal:1}]);
});


test("learner HTML5 player plays, seeks with a byte range, then unloads on entitlement revocation",async({page})=>{
 const enrollmentID="77777777-7777-4777-8777-777777777777";
 const streamPath="/v1/academy/enrollments/"+enrollmentID+"/lessons/"+lessonID+"/videos/"+mediaID+"/stream";
 let manifestReads=0;
 let revoked=false;
 let rangeRequests=0;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request();
  const url=new URL(request.url());
  const path=url.pathname;
  const method=request.method();
  let status=200;
  let data:unknown={};
  let contentType="application/json";
  let responseHeaders:Record<string,string>={};
  let responseBody:string|Buffer=JSON.stringify(data);
  if(method==="GET"&&path==="/v1/academy/courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-enrollments")data={enrollments:[{
   id:enrollmentID,version_id:versionID,course_id:courseID,course_title:"Проверочный курс",
   version_no:1,state:"ACTIVE",access_state:"ACTIVE"
  }]};
  else if(method==="GET"&&path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(method==="GET"&&path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(method==="GET"&&path==="/v1/academy/me/course-access")data={course_ids:[courseID]};
  else if(method==="GET"&&path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/learn"){
   data={lessons:[{id:lessonID,module_id:moduleID,title:"Урок с видео",kind:"VIDEO",ordinal:1,available:true}]};
  }else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/attempts"){
   data={attempts:[]};
  }else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/cohorts"){
   data={cohorts:[]};
  }else if(method==="GET"&&path==="/v1/academy/me/appeals"){
   data={appeals:[]};
  }else if(method==="GET"&&path===streamPath){
   contentType="video/webm";
   const range=request.headers()["range"];
   responseHeaders={"Accept-Ranges":"bytes","Cache-Control":"private, no-store"};
   if(range){
    rangeRequests++;
    const match=/^bytes=([0-9]+)-([0-9]*)$/.exec(range);
    if(!match){
     status=416;
     responseHeaders["Content-Range"]="bytes */"+learnerVideoFixture.length;
     responseBody="";
     contentType="text/plain";
    }else{
     const first=Number(match[1]);
     const last=Math.min(match[2]?Number(match[2]):learnerVideoFixture.length-1,learnerVideoFixture.length-1);
     if(!Number.isSafeInteger(first)||!Number.isSafeInteger(last)||first<0||last<first||first>=learnerVideoFixture.length){
      status=416;
      responseHeaders["Content-Range"]="bytes */"+learnerVideoFixture.length;
      responseBody="";
      contentType="text/plain";
     }else{
      status=206;
      responseHeaders["Content-Range"]="bytes "+first+"-"+last+"/"+learnerVideoFixture.length;
      responseBody=learnerVideoFixture.subarray(first,last+1);
      responseHeaders["Content-Length"]=String((responseBody as Buffer).length);
     }
    }
   }else{
    responseHeaders["Content-Length"]=String(learnerVideoFixture.length);
    responseBody=learnerVideoFixture;
   }
  }else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/lessons/"+lessonID+"/videos"){
   manifestReads++;
   if(revoked){
    status=403;data={code:"ENTITLEMENT_REVOKED"};
   }else{
    data={videos:[{id:mediaID,title:"Урок 1.webm",ordinal:1}]};
   }
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  if(contentType==="application/json")responseBody=JSON.stringify(data);
  await route.fulfill({status,contentType,headers:responseHeaders,body:responseBody});
 });

 await page.goto("/academy");
 await page.getByRole("button",{name:"Моё обучение"}).click();
 await page.getByRole("button",{name:"Продолжить",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toBeVisible();
 const video=page.locator("video");
 await expect(video).toHaveAttribute("src",streamPath);
 await video.evaluate(element=>{
  (window as unknown as {__academyVideo?:HTMLVideoElement}).__academyVideo=element as HTMLVideoElement;
 });
 await video.evaluate(async element=>{
  const player=element as HTMLVideoElement;
  player.muted=true;
  await player.play();
 });
 await page.waitForFunction(()=>{
  const player=document.querySelector("video");
  return player instanceof HTMLVideoElement&&player.readyState>=2&&player.duration>=1.9;
 });
 await video.evaluate(element=>new Promise<void>((resolve,reject)=>{
  const player=element as HTMLVideoElement;
  const timeout=window.setTimeout(()=>reject(new Error("video seek timed out")),5000);
  player.addEventListener("seeked",()=>{
   window.clearTimeout(timeout);resolve();
  },{once:true});
  player.currentTime=1;
 }));
 expect(await video.evaluate(element=>(element as HTMLVideoElement).currentTime)).toBeGreaterThanOrEqual(0.9);

 const rangeProof=await page.evaluate(async path=>{
  const response=await fetch(path,{headers:{Range:"bytes=0-127"}});
  const body=await response.arrayBuffer();
  return {status:response.status,contentRange:response.headers.get("content-range"),length:body.byteLength};
 },streamPath);
 expect(rangeProof).toEqual({status:206,contentRange:"bytes 0-127/"+learnerVideoFixture.length,length:128});
 expect(rangeRequests).toBeGreaterThan(0);

 expect(await video.evaluate(element=>!(element as HTMLVideoElement).paused)).toBe(true);
 revoked=true;
 await expect(page.getByRole("alert").filter({
  hasText:"Доступ к видео не подтверждён или был отозван"
 })).toBeVisible({timeout:8000});
 const state=await page.evaluate(()=>{
  const element=(window as unknown as {__academyVideo?:HTMLVideoElement}).__academyVideo;
  return {hasSource:element?.hasAttribute("src")??false,paused:element?.paused??false};
 });
 expect(state).toEqual({hasSource:false,paused:true});
 await expect(video).toHaveCount(0);
 expect(manifestReads).toBeGreaterThanOrEqual(2);
 const rendered=await page.locator("body").innerText();
 expect(rendered).not.toContain("downloader.disk.yandex");
 expect(rendered).not.toContain("storage.yandex.net");
});


test("learner video error is visible and retry revalidates entitlement before restoring the protected stream",async({page})=>{
 const enrollmentID="77777777-7777-4777-8777-777777777778";
 const streamPath="/v1/academy/enrollments/"+enrollmentID+"/lessons/"+lessonID+"/videos/"+mediaID+"/stream";
 let manifestReads=0;
 let streamAttempts=0;
 let retryRequested=false;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname,method=request.method();
  let status=200,contentType="application/json",data:unknown={},responseBody:string|Buffer="";
  const headers:Record<string,string>={};
  if(method==="GET"&&path==="/v1/academy/courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-courses")data={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-enrollments")data={enrollments:[{
   id:enrollmentID,version_id:versionID,course_id:courseID,course_title:"Проверочный курс",
   version_no:1,state:"ACTIVE",access_state:"ACTIVE"
  }]};
  else if(method==="GET"&&path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(method==="GET"&&path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(method==="GET"&&path==="/v1/academy/me/course-access")data={course_ids:[courseID]};
  else if(method==="GET"&&path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/learn"){
   data={lessons:[{id:lessonID,module_id:moduleID,title:"Урок с видео",kind:"VIDEO",ordinal:1,available:true}]};
  }else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/attempts")data={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/cohorts")data={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/me/appeals")data={appeals:[]};
  else if(method==="GET"&&path==="/v1/academy/enrollments/"+enrollmentID+"/lessons/"+lessonID+"/videos"){
   manifestReads++;
   data={videos:[{id:mediaID,title:"Урок 1.webm",ordinal:1}]};
  }else if(method==="GET"&&path===streamPath){
   streamAttempts++;
   if(!retryRequested){
    // Keep the provider unavailable until the learner explicitly retries.
    // This avoids browser-specific automatic retries turning the first
    // failure into a false-positive success.
    status=503;contentType="text/plain";responseBody="temporary provider failure";
   }else{
    contentType="video/webm";
    headers["Accept-Ranges"]="bytes";
    headers["Cache-Control"]="private, no-store";
    const range=request.headers()["range"];
    if(range){
     const match=/^bytes=([0-9]+)-([0-9]*)$/.exec(range);
     if(!match){
      status=416;contentType="text/plain";
      headers["Content-Range"]="bytes */"+learnerVideoFixture.length;
      responseBody="";
     }else{
      const first=Number(match[1]);
      const last=Math.min(match[2]?Number(match[2]):learnerVideoFixture.length-1,learnerVideoFixture.length-1);
      if(!Number.isSafeInteger(first)||!Number.isSafeInteger(last)||first<0||last<first||first>=learnerVideoFixture.length){
       status=416;contentType="text/plain";
       headers["Content-Range"]="bytes */"+learnerVideoFixture.length;
       responseBody="";
      }else{
       status=206;
       headers["Content-Range"]="bytes "+first+"-"+last+"/"+learnerVideoFixture.length;
       responseBody=learnerVideoFixture.subarray(first,last+1);
       headers["Content-Length"]=String((responseBody as Buffer).length);
      }
     }
    }else{
     headers["Content-Length"]=String(learnerVideoFixture.length);
     responseBody=learnerVideoFixture;
    }
   }
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  if(contentType==="application/json")responseBody=JSON.stringify(data);
  await route.fulfill({status,contentType,headers,body:responseBody});
 });

 await page.goto("/academy");
 await page.getByRole("button",{name:"Моё обучение"}).click();
 await page.getByRole("button",{name:"Продолжить",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toBeVisible();
 const video=page.locator("video");
 await expect(video).toHaveCount(1);
 // preload="none" is intentional in the learner player: explicitly start
 // playback so the browser makes the first protected stream request.
 await video.evaluate(element=>{
  const player=element as HTMLVideoElement;
  player.muted=true;
  void player.play().catch(()=>undefined);
 });
 await expect.poll(()=>streamAttempts).toBeGreaterThan(0);
 await expect(page.getByRole("button",{name:"Повторить загрузку видео"})).toBeVisible();
 await expect(video).not.toHaveAttribute("src",streamPath);
 const attemptsBeforeRetry=streamAttempts;
 retryRequested=true;
 await page.getByRole("button",{name:"Повторить загрузку видео"}).click();
 await expect(video).toHaveAttribute("src",streamPath);
 // The retry action revalidates the manifest asynchronously. preload="none"
 // will not fetch the restored source until playback is explicitly requested.
 await video.evaluate(async element=>{
  const player=element as HTMLVideoElement;
  player.muted=true;
  await player.play();
 });
 await expect.poll(()=>streamAttempts).toBeGreaterThan(attemptsBeforeRetry);
 await page.waitForFunction(()=>{
  const player=document.querySelector("video");
  return player instanceof HTMLVideoElement&&player.readyState>=2;
 });
 await expect(page.getByRole("button",{name:"Повторить загрузку видео"})).toHaveCount(0);
 expect(manifestReads).toBeGreaterThanOrEqual(2);
 expect(await video.evaluate(element=>(element as HTMLVideoElement).paused)).toBe(false);
 const rendered=await page.locator("body").innerText();
 expect(rendered).not.toContain("downloader.disk.yandex");
 expect(rendered).not.toContain("storage.yandex.net");
});
