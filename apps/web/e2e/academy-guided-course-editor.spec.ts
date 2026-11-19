import {expect,test} from "@playwright/test";

const courseId="11111111-1111-4111-8111-111111111111";
const versionId="22222222-2222-4222-8222-222222222222";
const moduleId="33333333-3333-4333-8333-333333333333";
const lessonId="44444444-4444-4444-8444-444444444444";
const course={id:courseId,version_id:versionId,version_no:1,product_id:courseId,
 title:"Курс для проверки конструктора",description:"Настоящий редактор",state:"DRAFT"};

test("guided course editor connects module, lesson, video, schedule, rights and publish to canonical APIs",async({page})=>{
 const calls:{method:string;path:string;body:unknown}[]=[];
 let modules:{id:string;version_id:string;ordinal:number;title:string}[]=[];
 let publishingApproved=false;
 let rightsConfirmed=false;
 let refreshUnavailableAfterPublish=false;
 let lessons:{id:string;module_id:string;title:string;ordinal:number;kind:string}[]=[];
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request(),path=new URL(req.url()).pathname,method=req.method();
  const body=method==="POST"?req.postDataJSON():undefined;
  let data:unknown={};let status=200;
  if(method==="GET"){
   if(path==="/v1/academy/courses"){
    if(refreshUnavailableAfterPublish){status=503;data={code:"ACADEMY_UNAVAILABLE"};}
    else data={courses:[]};
   }
   else if(path==="/v1/academy/my-courses")data={courses:[course]};
   else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
   else if(path==="/v1/academy/review/attempts")data={attempts:[]};
   else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
   else if(path==="/v1/academy/author/eligible-products")data={products:[]};
   else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
   else if(path==="/v1/academy/me/certificates")data={certificates:[]};
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/outline`)
    data={state:"DRAFT",modules,lessons};
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/rights/original`)
    data={confirmed:rightsConfirmed,provenance_ref:rightsConfirmed?"author-original-materials-reference":""};
   else if(path==="/v1/academy/author/cloud-media")data={media:[]};
   else if(path===`/v1/academy/author/courses/${courseId}/versions/${versionId}/media`)
    data={assignments:[]};
   else{status=501;data={code:"UNMOCKED",path};}
  }else if(method==="POST"){
   calls.push({method,path,body});
   if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/modules`){
    modules=[{id:moduleId,version_id:versionId,title:(body as {title:string}).title,ordinal:1}];
    data=modules[0];
   }else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/modules/${moduleId}/lessons`){
    lessons=[{id:lessonId,module_id:moduleId,title:(body as {title:string}).title,ordinal:1,kind:(body as {kind:string}).kind}];
    data=lessons[0];
   }else if(path===`/v1/academy/lessons/${lessonId}/schedule`)data={state:"SAVED"};
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/rights/original`){
    rightsConfirmed=true;data={id:"rights-proof"};
   }
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/publish`){
    if(!publishingApproved){status=409;data={code:"ACADEMY_STATE_CONFLICT",message_safe:"Содержание изменилось после подтверждения прав"};}
    else data={...course,state:"PUBLISHED"};
   }
   else{status=501;data={code:"UNMOCKED_MUTATION",path};}
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses");
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 const tree=page.getByRole("region",{name:"Дерево курса"});
 const action=page.getByRole("region",{name:"Действия с курсом"});
 await expect(tree.getByText("Пока нет модулей")).toBeVisible();
 await tree.getByRole("button",{name:"Создать первый модуль"}).click();
 await action.locator("#academy-module").fill("Основы");
 await action.getByRole("button",{name:"Сохранить модуль"}).click();
 await expect(tree.getByRole("button",{name:/Основы/})).toBeVisible();
 await expect(action.getByRole("heading",{name:"Добавить урок"})).toBeVisible();
 const editorWidth=await page.evaluate(()=>({viewport:innerWidth,document:document.documentElement.scrollWidth}));
 expect(editorWidth.document).toBeLessThanOrEqual(editorWidth.viewport+1);
 await action.locator("#academy-lesson-title").fill("Введение");
 await action.getByRole("button",{name:"Сохранить урок"}).click();
 await expect(page.locator("#academy-media-editor")).toBeVisible();
 await expect(page.getByLabel("К какому уроку добавить видео?")).toHaveValue(lessonId);
 await page.getByRole("button",{name:"1. Уроки"}).click();
 await expect(tree).toContainText("1. Введение");
 const dimensions=await tree.evaluate(element=>{
  const name=element.querySelector(".academy-lesson-name") as HTMLElement;
  const video=element.querySelector(".academy-lesson-video") as HTMLButtonElement;
  const module=element.querySelector(".academy-module-heading") as HTMLButtonElement;
  return {name:name.getBoundingClientRect().width,video:video.getBoundingClientRect().width,
   moduleColor:getComputedStyle(module).backgroundColor,
   videoColor:getComputedStyle(video).backgroundColor};
 });
 expect(dimensions.name).toBeGreaterThan(110);
 expect(dimensions.video).toBeLessThan(130);
 expect(dimensions.moduleColor).not.toBe("rgb(22, 63, 49)");
 expect(dimensions.videoColor).not.toBe("rgb(22, 63, 49)");
 await tree.getByRole("button",{name:"Видео →"}).click();
 await expect(page.getByRole("button",{name:"2. Видео"})).toHaveAttribute("aria-current","step");
 await page.getByRole("button",{name:"1. Уроки"}).click();
 await page.getByText("Расписание и ограничения уроков").click();
 const schedule=page.getByRole("group",{name:"Расписание и лимиты уроков"});
 await expect(schedule.getByRole("button",{name:"Сохранить расписание"})).toBeVisible();
 await schedule.locator('input[name="release"]').fill("2026-12-04T10:00");
 await schedule.getByRole("button",{name:"Сохранить расписание"}).click();
 await expect(page.getByRole("status").filter({hasText:"Расписание урока сохранено."})).toBeVisible();
 await page.getByRole("button",{name:"3. Публикация"}).click();
 await expect(page.getByRole("heading",{name:"2. Подтвердите авторские права"})).toBeVisible();
 const rights=page.getByRole("region",{name:"Декларация авторских прав"});
 await rights.getByRole("button",{name:"Подтвердить права"}).click();
 await expect(rights.getByRole("status")).toContainText("Укажите источник материалов");
 expect(calls.filter(c=>c.path.endsWith("/rights/original"))).toHaveLength(0);
 // A real unpublished course with no attestation must guide the author rather
 // than send an irreversible POST or show the old multi-sentence server error.
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Укажите источник материалов");
 expect(calls.filter(c=>c.path.endsWith("/publish"))).toHaveLength(0);

 await rights.locator("#academy-original-rights").fill("author-original-materials-reference");
 await rights.getByRole("checkbox").check();
 await rights.getByRole("button",{name:"Подтвердить права"}).click();
 await expect(rights.getByRole("status")).toContainText("Права подтверждены");
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Содержание изменилось после подтверждения прав");
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Публикация не выполнена");
 await expect(page.getByRole("button",{name:"Опубликовать курс"})).toBeVisible();
 publishingApproved=true;
 refreshUnavailableAfterPublish=true; // POST succeeds; follow-up GET temporarily fails.
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.getByRole("status").filter({hasText:"Курс опубликован"})).toBeVisible();
 await expect(page.locator(".academy-publish-submit")).toHaveCount(0);
 await expect(page.getByRole("status").filter({hasText:"Курс опубликован"})).toBeVisible();
 const paths=calls.map(c=>c.path);
 for(const segment of ["/modules","/lessons","/schedule","/rights/original","/publish"]){
  expect(paths.filter(path=>path.endsWith(segment))).toHaveLength(segment==="/publish"?2:1);
 }
 expect(calls.find(c=>c.path.endsWith("/lessons"))?.body).toMatchObject({kind:"VIDEO",body:"",ordinal:1});
 expect(calls.find(c=>c.path.endsWith("/rights/original"))?.body).toMatchObject({acknowledged:true});
 const dims=await page.evaluate(()=>({viewport:innerWidth,document:document.documentElement.scrollWidth}));
 expect(dims.document).toBeLessThanOrEqual(dims.viewport+1);
});

test("publishing accepts short rights source and saves explicit consent through canonical readback",async({page})=>{
 const calls:string[]=[];
 let stored=false;
 const lesson={id:lessonId,module_id:moduleId,title:"Введение",kind:"READING",ordinal:1};
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request(),path=new URL(req.url()).pathname,method=req.method();
  let status=200;let data:unknown={};
  if(method==="GET"){
   if(path==="/v1/academy/courses")data={courses:[]};
   else if(path==="/v1/academy/my-courses")data={courses:[course]};
   else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
   else if(path==="/v1/academy/review/attempts")data={attempts:[]};
   else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
   else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
   else if(path==="/v1/academy/me/certificates")data={certificates:[]};
   else if(path.endsWith("/outline"))data={state:"DRAFT",modules:[{id:moduleId,version_id:versionId,ordinal:1,title:"Основы"}],lessons:[lesson]};
   else if(path.endsWith("/rights/original"))data={confirmed:stored,provenance_ref:stored?"№1":""};
   else{status=501;data={code:"UNMOCKED",path};}
  }else if(method==="POST"){
   calls.push(path);
   if(path.endsWith("/rights/original")){stored=true;data={id:"rights-attestation"};}
   else if(path.endsWith("/publish"))data={...course,state:"PUBLISHED"};
   else{status=501;data={code:"UNMOCKED_MUTATION",path};}
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses");
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await page.getByRole("button",{name:"3. Публикация"}).click();
 const rights=page.getByRole("region",{name:"Декларация авторских прав"});
 const input=rights.locator("#academy-original-rights");
 await expect(input).not.toHaveAttribute("minlength");
 await input.fill("   ");
 await rights.getByRole("checkbox").check();
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Укажите источник материалов");
 expect(calls).toHaveLength(0);
 await input.fill("№1");
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.getByRole("status").filter({hasText:"Курс опубликован"})).toBeVisible();
 expect(calls.filter(p=>p.endsWith("/rights/original"))).toHaveLength(1);
 expect(calls.filter(p=>p.endsWith("/publish"))).toHaveLength(1);
});

test("reconfirming an already signed source uses immutable server evidence and publishes",async({page})=>{
 const immutableSource="исходные материалы";
 let rightsPosts=0,publicationPosts=0;
 const fixture={...course};
 const lesson={id:lessonId,module_id:moduleId,title:"Содержание",kind:"READING",ordinal:1};
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request(),path=new URL(req.url()).pathname,method=req.method();
  let status=200;let data:unknown={};
  if(method==="GET"){
   if(path==="/v1/academy/courses")data={courses:[]};
   else if(path==="/v1/academy/my-courses")data={courses:[fixture]};
   else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
   else if(path==="/v1/academy/review/attempts")data={attempts:[]};
   else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
   else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
   else if(path==="/v1/academy/me/certificates")data={certificates:[]};
   else if(path.endsWith("/outline"))data={state:"DRAFT",modules:[{id:moduleId,version_id:versionId,title:"Первый модуль",ordinal:1}],lessons:[lesson]};
   else if(path.endsWith("/rights/original"))data={confirmed:true,provenance_ref:immutableSource};
   else {status=501;data={code:"UNMOCKED",path};}
  }else if(method==="POST"){
   if(path.endsWith("/rights/original")){rightsPosts++;data={id:"already-signed",provenance_ref:immutableSource};}
   else if(path.endsWith("/publish")){publicationPosts++;data={...fixture,state:"PUBLISHED"};}
   else {status=501;data={code:"UNMOCKED_POST",path};}
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses");
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await page.getByRole("button",{name:"3. Публикация"}).click();
 const rights=page.getByRole("region",{name:"Декларация авторских прав"});
 await expect(rights.locator("#academy-original-rights")).toHaveValue(immutableSource);
 await rights.locator("#academy-original-rights").fill("№1");
 await rights.getByRole("button",{name:"Подтвердить права"}).click();
 await expect(rights.getByRole("status")).toContainText("ранее сохранённый источник");
 await expect(rights.locator("#academy-original-rights")).toHaveValue(immutableSource);
 await page.getByRole("button",{name:"Опубликовать курс"}).click();
 await expect(page.getByRole("status").filter({hasText:"Курс опубликован"})).toBeVisible();
 expect(rightsPosts).toBe(1);
 expect(publicationPosts).toBe(1);
});
