import {expect,test} from "@playwright/test";

const courseId="11111111-1111-4111-8111-111111111111";
const versionId="22222222-2222-4222-8222-222222222222";
const moduleId="33333333-3333-4333-8333-333333333333";
const lessonId="44444444-4444-4444-8444-444444444444";
const course={id:courseId,version_id:versionId,version_no:1,product_id:courseId,
 title:"Курс по гипнозу",description:"Практика и теория",state:"DRAFT"};

test("author workspace routes every primary navigation and editor step to canonical functions",async({page})=>{
 const requests:string[]=[];
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request();const pathname=new URL(req.url()).pathname;
  requests.push(req.method()+" "+pathname);
  let data:unknown={};
  let status=200;
  if(req.method()==="GET"){
   if(pathname==="/v1/academy/courses")data={courses:[]};
   else if(pathname==="/v1/academy/my-courses")data={courses:[course]};
   else if(pathname==="/v1/academy/my-enrollments")data={enrollments:[]};
   else if(pathname==="/v1/academy/review/attempts")data={attempts:[]};
   else if(pathname==="/v1/academy/author/report")data={courses:[{course_id:courseId,title:course.title,enrollments:9,completed:2,active_access:8,pending_assessments:0,passed_assessments:1}],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(pathname==="/v1/academy/author/cohorts")data={cohorts:[]};
   else if(pathname==="/v1/academy/author/eligible-products")data={products:[]};
   else if(pathname==="/v1/academy/me/course-access")data={course_ids:[]};
   else if(pathname==="/v1/academy/me/certificates")data={certificates:[]};
   else if(pathname==="/v1/academy/courses/"+courseId+"/versions/"+versionId+"/outline")
    data={state:"DRAFT",modules:[{id:moduleId,version_id:versionId,ordinal:1,title:"Введение"}],
     lessons:[{id:lessonId,module_id:moduleId,title:"Введение",kind:"VIDEO",ordinal:1}]};
   else if(pathname==="/v1/academy/author/cloud-media")data={media:[]};
   else if(pathname==="/v1/academy/author/courses/"+courseId+"/versions/"+versionId+"/media")data={assignments:[]};
   else {status=501;data={code:"UNMOCKED",pathname};}
  }else {status=501;data={code:"UNMOCKED_MUTATION",pathname};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Кабинет автора"})).toBeVisible();
 await expect(page.getByText("9",{exact:true})).toBeVisible();
 await expect(page.getByRole("button",{name:"＋ Создать курс"})).toBeVisible();
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Ваши курсы"})).toBeVisible();
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await expect(page.getByRole("heading",{name:"Курс по гипнозу"})).toBeVisible();
 await expect(page.getByRole("button",{name:"1. Уроки"})).toHaveAttribute("aria-current","step");
 await page.getByRole("button",{name:"2. Видео"}).click();
 await expect(page.locator("#academy-media-editor")).toBeVisible();
 await page.getByRole("button",{name:"3. Публикация"}).click();
 await expect(page.getByRole("heading",{name:"1. Проверьте содержание курса"})).toBeVisible();
 await expect(page.getByRole("heading",{name:"2. Подтвердите авторские права"})).toBeVisible();
 await page.getByRole("button",{name:"Проверить уроки"}).click();
 await page.getByRole("button",{name:"＋ Модуль"}).click();
 await expect(page.locator("#academy-module")).toBeVisible();
 await page.getByRole("button",{name:"← Все курсы"}).click();
 await page.getByRole("button",{name:"＋ Создать курс"}).click();
 await expect(page.locator("#academy-title")).toBeVisible();
 await page.getByRole("button",{name:"Ученики и продажи"}).click();
 await expect(page.getByText("Зачислено: 9",{exact:false})).toBeVisible();
 await page.getByRole("button",{name:"Проверка работ"}).click();
 await expect(page.getByRole("heading",{name:"Проверка заданий преподавателем"})).toBeVisible();
 await page.getByRole("button",{name:"Каталог"}).click();
 await expect(page.getByRole("heading",{name:"Каталог"})).toBeVisible();
 await page.getByRole("button",{name:"Моё обучение"}).click();
 await expect(page.getByRole("heading",{name:"Моё обучение",exact:true})).toBeVisible();
 expect(requests).toContain("GET /v1/academy/my-courses");
 expect(requests).toContain("GET /v1/academy/author/report");
 expect(requests).toContain("GET /v1/academy/courses/"+courseId+"/versions/"+versionId+"/outline");
});


test("create course button writes through the existing Academy drafts endpoint once",async({page})=>{
 let drafts=0;
 let saved=false;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request();const path=new URL(request.url()).pathname;
  let response:unknown={};let status=200;
  if(request.method()==="POST"&&path==="/v1/academy/drafts"){
   drafts++;
   const body=request.postDataJSON() as {title:string;description:string};
   if(body.title!=="Новый курс"||body.description!=="Описание"){status=400;response={code:"BAD_DRAFT"};}
   else{saved=true;response={...course,title:body.title,description:body.description};}
  }else if(request.method()==="GET"){
   if(path==="/v1/academy/courses")response={courses:[]};
   else if(path==="/v1/academy/my-courses")response={courses:saved?[{...course,title:"Новый курс",description:"Описание"}]:[]};
   else if(path==="/v1/academy/my-enrollments")response={enrollments:[]};
   else if(path==="/v1/academy/review/attempts")response={attempts:[]};
   else if(path==="/v1/academy/author/report")response={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(path==="/v1/academy/author/cohorts")response={cohorts:[]};
   else if(path==="/v1/academy/author/eligible-products")response={products:[]};
   else if(path==="/v1/academy/me/course-access")response={course_ids:[]};
   else if(path==="/v1/academy/me/certificates")response={certificates:[]};
   else if(path==="/v1/academy/courses/"+courseId+"/versions/"+versionId+"/outline")response={state:"DRAFT",modules:[],lessons:[]};
   else{status=501;response={code:"UNMOCKED",path};}
  }else{status=501;response={code:"UNMOCKED_MUTATION",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(response)});
 });
 await page.goto("/academy");
 await page.getByRole("button",{name:"＋ Создать курс"}).click();
 await expect(page.locator("#academy-title")).toBeVisible();
 await expect(page.getByRole("link",{name:"Настроить организацию и продажи"})).toHaveAttribute("href","/organization");
 await page.locator("#academy-title").fill("Новый курс");
 await page.locator("#academy-description").fill("Описание");
 await page.locator("#academy-author").getByRole("button",{name:"Создать курс",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Новый курс"})).toBeVisible();
 await page.getByRole("button",{name:"＋ Модуль"}).click();
 await expect(page.locator("#academy-module")).toBeVisible();
 expect(drafts).toBe(1);
});
