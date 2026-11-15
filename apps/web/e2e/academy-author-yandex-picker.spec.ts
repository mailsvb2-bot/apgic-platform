import {expect, test} from "@playwright/test";

// This browser test locks the author-facing sequence after a successful
// Yandex OAuth callback. Provider OAuth and Yandex's own API are deliberately
// mocked here; this is a UI regression test, not live-provider acceptance.
const courseID="11111111-1111-4111-8111-111111111111";
const versionID="22222222-2222-4222-8222-222222222222";
const moduleID="33333333-3333-4333-8333-333333333333";
const lessonID="44444444-4444-4444-8444-444444444444";
const mediaID="55555555-5555-4555-8555-555555555555";
const diskFilePath="disk:/Лекции/Урок 1.mp4";

test("author resumes the selected course and lesson, imports Disk video, and saves its lesson mapping",async({page})=>{
 const course={
  id:courseID,product_id:"66666666-6666-4666-8666-666666666666",
  title:"Проверочный курс",description:"",state:"DRAFT",version_id:versionID,version_no:1
 };
 const media: {id:string;title:string;provider:string;provider_file_ref:string;active:boolean}[]=[];
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
  else if(method==="GET"&&path===`/v1/academy/courses/${courseID}/versions/${versionID}/outline"){
   data={state:"DRAFT",modules:[{id:moduleID,version_id:versionID,title:"Модуль 1",ordinal:1}],
    lessons:[{id:lessonID,module_id:moduleID,title:"Урок 1",kind:"VIDEO",ordinal:1}]};
  }else if(method==="GET"&&path==="/v1/academy/author/cloud-media")data={media};
  else if(method==="GET"&&path===`/v1/academy/author/courses/${courseID}/versions/${versionID}/media"){
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
   for(const item of body.items)media.push({id:mediaID,title:item.title,provider:"YANDEX_DISK",provider_file_ref:item.provider_file_ref,active:true});
   status=201;data={media:media.slice()};
  }else if(method==="POST"&&path===`/v1/academy/courses/${courseID}/versions/${versionID}/media/assign"){
   const body=request.postDataJSON() as {items:{lesson_id:string;media_id:string;ordinal:number}[]};
   savedPayload=body.items;
   assignments=body.items;
   data={state:"SAVED"};
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });

 await page.goto(`/academy?cloud_connected=yandex&course_id=${courseID}&version_id=${versionID}&lesson_id=${lessonID}`);
 await expect(page.getByRole("heading",{name:"Редактор: Проверочный курс — версия 1"})).toBeVisible();
 await expect(page.getByText("Шаг 1. Курс выбран. Шаг 2. Добавьте видеоуроки ниже. Шаг 3. Выберите для каждого урока видео с Яндекс Диска.")).toBeVisible();
 await expect(page.getByLabel("К какому уроку добавить видео?")).toHaveValue(lessonID);
 await expect(page.getByRole("button",{name:"Папка: Лекции"})).toBeVisible();

 await page.getByRole("button",{name:"Папка: Лекции"}).click();
 await expect(page.getByRole("checkbox",{name:"Урок 1.mp4"})).toBeVisible();
 await page.getByRole("checkbox",{name:"Урок 1.mp4"}).check();
 await page.getByRole("button",{name:"Добавить к уроку (1)"}).click();
 await expect(page.getByText("Видео добавлены к выбранному уроку. Нажмите «Сохранить видео урока».")).toBeVisible();
 expect(importCalls).toBe(1);

 await page.getByRole("button",{name:"Сохранить видео урока"}).click();
 await expect(page.getByText("Распределение сохранено")).toBeVisible();
 expect(savedPayload).toEqual([{lesson_id:lessonID,media_id:mediaID,ordinal:1}]);
 expect(assignments).toEqual([{lesson_id:lessonID,media_id:mediaID,ordinal:1}]);
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
  else if(method==="GET"&&path===`/v1/academy/courses/${courseID}/versions/${versionID}/outline"){
   data={state:"DRAFT",modules:[{id:moduleID,version_id:versionID,title:"Модуль 1",ordinal:1}],lessons};
  }else if(method==="POST"&&path===`/v1/academy/courses/${courseID}/versions/${versionID}/modules/${moduleID}/lessons"){
   createdLessonBody=request.postDataJSON() as Record<string,unknown>;
   lessons.push({id:lessonID,module_id:moduleID,title:"Урок 1",kind:"VIDEO",ordinal:1});
   status=201;data=lessons[0];
  }else if(method==="GET"&&path==="/v1/academy/author/cloud-media")data={media};
  else if(method==="GET"&&path===`/v1/academy/author/courses/${courseID}/versions/${versionID}/media"){
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
  }else if(method==="POST"&&path===`/v1/academy/courses/${courseID}/versions/${versionID}/media/assign"){
   const body=request.postDataJSON() as {items:{lesson_id:string;media_id:string;ordinal:number}[]};
   savedAssignments=body.items;data={state:"SAVED"};
  }else{
   status=501;data={code:"UNMOCKED_ACADEMY_ROUTE",path,method};
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });

 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Мои курсы как автора"})).toBeVisible();
 await page.getByRole("button",{name:"Редактирование и содержание"}).click();
 await expect(page.locator("#academy-lesson-title")).toBeVisible();
 await page.locator("#academy-lesson-title").fill("Урок 1");
 // A previous text entry must never become a hidden media identifier.
 await page.locator("#academy-lesson-body").fill("Старый текстовый материал");
 await page.locator("#academy-lesson-kind").selectOption("VIDEO");
 await page.getByRole("button",{name:"Добавить урок",exact:true}).click();
 await expect(page.getByLabel("К какому уроку добавить видео?")).toBeVisible();
 expect(createdLessonBody?.kind).toBe("VIDEO");
 expect(createdLessonBody?.body).toBe("");
 expect(createdLessonBody?.media_asset_ref).toBeUndefined();

 await page.getByLabel("К какому уроку добавить видео?").selectOption(lessonID);
 await page.getByRole("button",{name:"Открыть Яндекс Диск"}).click();
 await page.getByRole("button",{name:"Папка: Лекции"}).click();
 await page.getByRole("checkbox",{name:"Урок 1.mp4"}).check();
 await page.getByRole("button",{name:"Добавить к уроку (1)"}).click();
 await expect(page.getByText("Видео добавлены к выбранному уроку. Нажмите «Сохранить видео урока».")).toBeVisible();
 await page.getByRole("button",{name:"Сохранить видео урока"}).click();
 await expect(page.getByText("Распределение сохранено")).toBeVisible();
 expect(savedAssignments).toEqual([{lesson_id:lessonID,media_id:mediaID,ordinal:1}]);
});
