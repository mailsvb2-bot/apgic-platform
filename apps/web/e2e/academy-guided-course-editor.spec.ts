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
 let lessons:{id:string;module_id:string;title:string;ordinal:number;kind:string}[]=[];
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request(),path=new URL(req.url()).pathname,method=req.method();
  const body=method==="POST"?req.postDataJSON():undefined;
  let data:unknown={};let status=200;
  if(method==="GET"){
   if(path==="/v1/academy/courses")data={courses:[]};
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
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/rights/original`)data={id:"rights-proof"};
   else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/publish`){
    if(!publishingApproved){status=409;data={code:"VIDEO_RELEASE_NOT_APPROVED",message_safe:"Доставка видео пока не подтверждена"};}
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
 await expect(page.getByRole("heading",{name:"2. Подтвердите права на материалы"})).toBeVisible();
 const rights=page.getByRole("region",{name:"Декларация авторских прав"});
 await rights.locator("#academy-original-rights").fill("author-original-materials-reference");
 await rights.getByRole("checkbox").check();
 await rights.getByRole("button",{name:"Зафиксировать права на текущую версию"}).click();
 await expect(rights.getByText("Подтверждение зафиксировано.")).toBeVisible();
 await expect(page.getByText(/сервер дополнительно требует проверенные файлы Яндекс Диска/)).toBeVisible();
 await page.getByRole("button",{name:"Опубликовать версию курса"}).click();
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Доставка видео пока не подтверждена");
 await expect(page.locator(".academy-publish-submit").getByRole("alert")).toContainText("Версия осталась черновиком");
 await expect(page.getByRole("button",{name:"Опубликовать версию курса"})).toBeVisible();
 publishingApproved=true;
 await page.getByRole("button",{name:"Опубликовать версию курса"}).click();
 await expect(page.getByRole("status").filter({hasText:"Версия опубликована"})).toBeVisible();
 await expect(page.locator(".academy-publish-submit")).toHaveCount(0);
 const paths=calls.map(c=>c.path);
 for(const segment of ["/modules","/lessons","/schedule","/rights/original","/publish"]){
  expect(paths.filter(path=>path.endsWith(segment))).toHaveLength(segment==="/publish"?2:1);
 }
 expect(calls.find(c=>c.path.endsWith("/lessons"))?.body).toMatchObject({kind:"VIDEO",body:"",ordinal:1});
 expect(calls.find(c=>c.path.endsWith("/rights/original"))?.body).toMatchObject({acknowledged:true});
 const dims=await page.evaluate(()=>({viewport:innerWidth,document:document.documentElement.scrollWidth}));
 expect(dims.document).toBeLessThanOrEqual(dims.viewport+1);
});
