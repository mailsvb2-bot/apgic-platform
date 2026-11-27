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
 await expect(page.getByRole("heading",{name:"Мои курсы",exact:true})).toBeVisible();
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

test("published-only author cannot open a nonexistent video editor",async({page})=>{
 const published={...course,state:"PUBLISHED"};
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  let data:unknown={};let status=200;
  if(path==="/v1/academy/courses")data={courses:[published]};
  else if(path==="/v1/academy/my-courses")data={courses:[published]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else {status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 const action=page.getByRole("button",{name:"▷ Видео: нужен черновик"});
 await expect(action).toBeDisabled();
 await expect(action).toHaveAttribute("title",/создайте новую версию/);
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await expect(page.getByRole("button",{name:"Новая версия"})).toBeVisible();
});

test("public catalog outage does not block author drafts or learner navigation",async({page})=>{
 const requests:string[]=[];
 await page.route("**/v1/academy/**",async route=>{
  const url=new URL(route.request().url()),path=url.pathname;
  requests.push(path);
  let status=200,data:unknown={};
  if(path==="/v1/academy/courses"){status=503;data={code:"CATALOG_UNAVAILABLE",message_safe:"Каталог на обслуживании"};}
  else if(path==="/v1/academy/my-courses")data={courses:[course]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/outline"))data={state:"DRAFT",modules:[],lessons:[]};
  else {status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Курс по гипнозу",exact:true})).toBeVisible();
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await expect(page.getByRole("button",{name:"Открыть редактор"})).toBeVisible();
 await page.getByRole("button",{name:"Каталог",exact:true}).click();
 await expect(page.locator(".academy-main-section [role=\"alert\"]")).toContainText("Каталог временно недоступен");
 await expect(page.getByText("Опубликованных курсов пока нет.")).toHaveCount(0);
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await expect(page.getByRole("heading",{name:"Курс по гипнозу"})).toBeVisible();
 expect(requests).toContain("/v1/academy/my-courses");
});


test("learner can open lessons when attempt-history API is temporarily unavailable",async({page})=>{
 const enrollment={id:"55555555-5555-4555-8555-555555555555",course_id:courseId,
  course_title:"Курс по гипнозу",version_id:versionId,version_no:1,state:"ACTIVE",access_state:"ACTIVE"};
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  let data:unknown={},status=200;
  if(path==="/v1/academy/courses")data={courses:[]};
  else if(path==="/v1/academy/my-courses")data={courses:[]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[enrollment]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[courseId]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/learn"))data={lessons:[{id:lessonId,module_id:moduleId,title:"Теория",kind:"READING",body:"Содержание урока",ordinal:1,available:true,completed:false}]};
  else if(path.endsWith("/attempts")){status=503;data={code:"ATTEMPTS_UNAVAILABLE"};}
  else if(path.endsWith("/cohorts"))data={cohorts:[]};
  else if(path==="/v1/academy/me/appeals")data={appeals:[]};
  else{status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await page.getByRole("button",{name:"Моё обучение",exact:true}).click();
 await page.getByRole("button",{name:"Продолжить"}).click();
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toBeVisible();
 await expect(page.getByRole("heading",{name:"Теория"})).toBeVisible();
 await expect(page.getByText("Содержание урока")).toBeVisible();
 await expect(page.getByRole("alert").filter({hasText:"История выполненных заданий временно недоступна"})).toBeVisible();
});


test("specialist reports deep link opens the canonical Academy results panel",async({page})=>{
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  let data:unknown={},status=200;
  if(path==="/v1/academy/courses"||path==="/v1/academy/my-courses")data={courses:[]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else {status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=reports");
 await expect(page.getByRole("button",{name:"Ученики и продажи"})).toHaveAttribute("aria-current","page");
 await expect(page.getByText("Кабинет автора",{exact:true})).toHaveCount(0);
});

test("published-only author starts a fresh draft instead of an immutable editor",async({page})=>{
 const published={...course,state:"PUBLISHED"};
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  let data:unknown={},status=200;
  if(path==="/v1/academy/courses"||path==="/v1/academy/my-courses")data={courses:[published]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else {status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await expect(page.getByRole("button",{name:"Продолжить редактирование"})).toHaveCount(0);
 await page.locator(".academy-feature").getByRole("button",{name:"Создать новый курс"}).click();
 await expect(page.locator("#academy-title")).toBeVisible();
 await expect(page.getByRole("heading",{name:"Ваши курсы"})).toBeVisible();
});


test("R8 revoked author and learner readbacks do not retain private report or enrollment",async({page})=>{
 let revoked=false;
 const enrollment={id:"55555555-5555-4555-8555-555555555555",course_id:courseId,
  course_title:"Закрытый курс ученика",version_id:versionId,version_no:1,
  state:"ACTIVE",access_state:"ACTIVE"};
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  if(revoked&&["/v1/academy/my-courses","/v1/academy/my-enrollments",
    "/v1/academy/review/attempts","/v1/academy/author/report",
    "/v1/academy/author/cohorts","/v1/academy/me/course-access",
    "/v1/academy/me/certificates"].includes(path)){
   return route.fulfill({status:401,json:{code:"CLIENT_SESSION_INVALID",message_safe:"Требуется повторный вход."}});
  }
  let data:unknown={};
  if(path==="/v1/academy/courses")data={courses:[]};
  else if(path==="/v1/academy/my-courses")data={courses:[course]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[enrollment]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[{
   course_id:courseId,title:"Частный доход автора",enrollments:3,completed:1,
   active_access:3,pending_assessments:0,passed_assessments:1
  }],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[courseId]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/learn"))data={lessons:[{id:lessonId,module_id:moduleId,title:"Закрытый учебный материал",kind:"READING",body:"Личные учебные заметки",ordinal:1,available:true,completed:false}]};
  else if(path.endsWith("/attempts"))data={attempts:[]};
  else if(path.endsWith("/cohorts"))data={cohorts:[]};
  else if(path==="/v1/academy/me/appeals")data={appeals:[]};
  else if(path==="/v1/academy/me/notifications")data={
   enabled:false,learning_updates:false,assessment_results:false,certificate_updates:false
  };
  else return route.fulfill({status:501,json:{code:"UNMOCKED",path}});
  return route.fulfill({status:200,json:data});
 });
 await page.goto("/academy?tab=reports");
 await expect(page.getByRole("heading",{name:"Частный доход автора"})).toBeVisible();
 await page.getByRole("button",{name:"Моё обучение"}).click();
 await expect(page.getByText("Закрытый курс ученика")).toBeVisible();
 await page.getByRole("button",{name:"Продолжить"}).click();
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toBeVisible();
 await expect(page.getByText("Закрытый учебный материал")).toBeVisible();
 revoked=true;
 await page.getByRole("button",{name:"Настроить уведомления"}).click();
 await expect(page.getByRole("alert").filter({hasText:"История обучения временно недоступна"})).toBeVisible();
 await expect(page.getByText("Закрытый курс ученика")).toHaveCount(0);
 await expect(page.getByText("Закрытый учебный материал")).toHaveCount(0);
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toHaveCount(0);
 await expect(page.getByText("Пока нет зачислений.")).toHaveCount(0);
 await page.getByRole("button",{name:"Ученики и продажи"}).click();
 await expect(page.getByRole("status").filter({hasText:"Отчёт пока недоступен"})).toBeVisible();
 await expect(page.getByText("Частный доход автора")).toHaveCount(0);
 await page.getByRole("button",{name:"Проверка работ"}).click();
 await expect(page.getByRole("alert").filter({hasText:"Очередь проверки работ недоступна"})).toBeVisible();
 await expect(page.getByRole("button",{name:"Зафиксировать оценку"})).toBeDisabled();
});


test("R8 editor cannot remain open after canonical author ownership readback fails",async({page})=>{
 let revoked=false;
 let newVersions=0;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;
  if(request.method()==="POST"&&path===`/v1/academy/courses/${courseId}/versions`){
   newVersions++;
   revoked=true;
   return route.fulfill({status:201,json:{...course,version_no:2}});
  }
  if(revoked&&path==="/v1/academy/my-courses")
   return route.fulfill({status:401,json:{code:"SESSION_EXPIRED",message_safe:"Требуется вход"}});
  let data:unknown={};
  if(path==="/v1/academy/courses")data={courses:[]};
  else if(path==="/v1/academy/my-courses")data={courses:[course]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/outline"))data={state:"DRAFT",modules:[],lessons:[]};
  else return route.fulfill({status:501,json:{code:"UNMOCKED",path}});
  return route.fulfill({status:200,json:data});
 });
 await page.goto("/academy?tab=courses");
 await page.getByRole("button",{name:"Новая версия"}).click();
 await expect(page.getByRole("alert").filter({hasText:"Не удалось загрузить ваши курсы"})).toBeVisible();
 await expect(page.getByRole("heading",{name:"Курс по гипнозу"})).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Создать первый модуль"})).toHaveCount(0);
 expect(newVersions).toBe(1);
});


test("R8 Continue makes one authorized lesson read and clears private lessons on later denial",async({page})=>{
 const enrollment={id:"55555555-5555-4555-8555-555555555555",course_id:courseId,
  course_title:"Защищённый курс",version_id:versionId,version_no:1,state:"ACTIVE",access_state:"ACTIVE"};
 let learnCalls=0;
 await page.route("**/v1/academy/**",async route=>{
  const path=new URL(route.request().url()).pathname;
  if(path===`/v1/academy/enrollments/${enrollment.id}/learn`){
   learnCalls++;
   if(learnCalls===2)return route.fulfill({status:403,json:{
    code:"ACADEMY_ENTITLEMENT_REQUIRED",message_safe:"Доступ к урокам отозван."
   }});
   return route.fulfill({status:200,json:{lessons:[{
    id:lessonId,module_id:moduleId,title:"Защищённый урок",kind:"READING",
    body:"Приватное содержимое ученика",ordinal:1,available:true,completed:false
   }]}});
  }
  let data:unknown={};
  if(path==="/v1/academy/courses"||path==="/v1/academy/my-courses")data={courses:[]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[enrollment]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[courseId]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/attempts"))data={attempts:[]};
  else if(path.endsWith("/cohorts"))data={cohorts:[]};
  else if(path==="/v1/academy/me/appeals")data={appeals:[]};
  else return route.fulfill({status:501,json:{code:"UNMOCKED",path}});
  return route.fulfill({status:200,json:data});
 });
 await page.goto("/academy?tab=learning");
 const continueButton=page.getByRole("button",{name:"Продолжить"});
 await expect(continueButton).toBeVisible();
 await continueButton.click();
 await expect(page.getByText("Приватное содержимое ученика")).toBeVisible();
 expect(learnCalls).toBe(1);
 await continueButton.click();
 await expect(page.getByRole("status").filter({hasText:"Доступ к урокам отозван."})).toBeVisible();
 await expect(page.getByText("Приватное содержимое ученика")).toHaveCount(0);
 await expect(page.getByRole("heading",{name:"Прохождение курса"})).toHaveCount(0);
 expect(learnCalls).toBe(2);
});


test("missing author session offers explicit onboarding before creating exactly one draft",async({page})=>{
 let session=false,created=0;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;
  let status=200,data:unknown={};
  if(path==="/v1/academy/session/start"&&request.method()==="POST"){
   session=true;data={session_ready:true,new_identity:true};
  }else if(path==="/v1/academy/courses")data={courses:[]};
  else if(!session){status=401;data={code:"CLIENT_SESSION_REQUIRED",message_safe:"Требуется сессия клиента."};}
  else if(path==="/v1/academy/my-courses")data={courses:created?[{...course,title:"гипноз",description:""}]:[]};
  else if(path==="/v1/academy/drafts"&&request.method()==="POST"){
   created++;data={...course,title:"гипноз",description:""};status=201;
  }else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path.endsWith("/outline"))data={state:"DRAFT",modules:[],lessons:[]};
  else if(path.endsWith("/rights/original"))data={confirmed:false,provenance_ref:""};
  else{status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses&create=1");
 await page.locator("#academy-title").fill("гипноз");
 await expect(page.getByRole("alert").filter({hasText:"нет сессии APGIC"})).toBeVisible();
 await expect(page.locator("#academy-author").getByRole("button",{name:"Создать курс",exact:true})).toBeDisabled();
 await expect(page.getByRole("link",{name:"подключите это устройство к своему аккаунту"})).toHaveAttribute("href","/account");
 expect(created).toBe(0);
 await page.getByRole("button",{name:"Начать новый аккаунт автора"}).click();
 await expect(page.getByRole("status").filter({hasText:"Сессия автора создана"})).toBeVisible();
 await page.locator("#academy-author").getByRole("button",{name:"Создать курс",exact:true}).click();
 await expect(page.getByRole("heading",{name:"гипноз",exact:true})).toBeVisible();
 expect(created).toBe(1);
});


test("owner previews a published course through read-only outline and protected video manifest",async({page})=>{
 const requests:string[]=[];
 const readingId="55555555-5555-4555-8555-555555555555";
 const mediaId="66666666-6666-4666-8666-666666666666";
 const published={...course,state:"PUBLISHED"};
 await page.route("**/v1/academy/**",async route=>{
  const req=route.request(),path=new URL(req.url()).pathname;
  requests.push(req.method()+" "+path);
  let data:unknown={};let status=200;
  if(req.method()!=="GET"){status=405;data={code:"NO_PREVIEW_MUTATIONS"};}
  else if(path==="/v1/academy/my-courses")data={courses:[published]};
  else if(path==="/v1/academy/courses")data={courses:[published]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/author/eligible-products")data={products:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path===`/v1/academy/courses/${courseId}/versions/${versionId}/outline`)data={state:"PUBLISHED",version_id:versionId,modules:[{id:moduleId,version_id:versionId,ordinal:1,title:"Основы"}],lessons:[{id:readingId,module_id:moduleId,title:"Теория",kind:"READING",ordinal:1,body:"Авторский материал"},{id:lessonId,module_id:moduleId,title:"Практика",kind:"VIDEO",ordinal:2}]};
  else if(path===`/v1/academy/author/versions/${versionId}/lessons/${lessonId}/videos`)data={videos:[{id:mediaId,title:"Видео автора",ordinal:1}]};
  else {status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy");
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await page.getByRole("button",{name:"Посмотреть курс",exact:true}).click();
 const preview=page.getByRole("region",{name:"Предпросмотр курса"});
 await expect(preview.getByText("Авторский материал",{exact:true})).toBeVisible();
 await preview.getByRole("button",{name:"Следующий урок →"}).click();
 await expect(preview.getByRole("heading",{name:"Практика",exact:true})).toBeVisible();
 await expect(preview.locator("video")).toHaveAttribute("src",`/v1/academy/author/versions/${versionId}/lessons/${lessonId}/videos/${mediaId}/stream`);
 await page.getByRole("button",{name:"Вернуться к редактированию"}).click();
 await expect(preview).toHaveCount(0);
 await expect(page.getByRole("region",{name:"Дерево курса"})).toBeVisible();
 expect(requests.every(request=>request.startsWith("GET "))).toBe(true);
});
