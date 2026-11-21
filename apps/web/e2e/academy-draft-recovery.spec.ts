import {expect,test,type Page} from "@playwright/test";

const draft={
 id:"11111111-1111-4111-8111-111111111111",
 product_id:"22222222-2222-4222-8222-222222222222",
 version_id:"33333333-3333-4333-8333-333333333333",
 version_no:1,title:"гипноз",description:"",state:"DRAFT"
};

async function mockAcademy(page:Page, create:(requestID:string)=>{status:number;data:unknown},
 outline:()=>{status:number;data:unknown}, saved:()=>boolean,
 requests:string[],courseTitle="гипноз"){
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;
  let status=200,data:unknown={};
  if(request.method()==="POST"&&path==="/v1/academy/drafts"){
   const payload=request.postDataJSON() as {title:string;request_id:string};
   expect(payload.title).toBe(courseTitle);
   expect(payload.request_id).toMatch(/^[0-9a-f-]{36}$/);
   requests.push(payload.request_id);
   const result=create(payload.request_id);status=result.status;data=result.data;
  }else if(path==="/v1/academy/courses")data={courses:[]};
  else if(path==="/v1/academy/my-courses")data={courses:saved()?[draft]:[]};
  else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
  else if(path==="/v1/academy/review/attempts")data={attempts:[]};
  else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
  else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
  else if(path==="/v1/academy/me/certificates")data={certificates:[]};
  else if(path===`/v1/academy/courses/${draft.id}/versions/${draft.version_id}/outline`){
   const result=outline();status=result.status;data=result.data;
  }else if(path.endsWith("/rights/original"))data={confirmed:false,provenance_ref:""};
  else{status=501;data={code:"UNMOCKED",path};}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses&create=1");
 await page.locator("#academy-title").fill(courseTitle);
}

const outlineOk=()=>({status:200,data:{state:"DRAFT",modules:[],lessons:[]}});

test("definitive draft rejection never locks author into ambiguous write recovery",async({page})=>{
 const requests:string[]=[];
 await mockAcademy(page,()=>({status:401,data:{code:"CLIENT_SESSION_REQUIRED",message_safe:"Сессия автора истекла."}}),
  outlineOk,()=>false,requests);
 const form=page.locator("#academy-author");
 await form.getByRole("button",{name:"Создать курс",exact:true}).click();
 await expect(page.getByRole("status").filter({hasText:"Курс не был создан"})).toBeVisible();
 await expect(form.getByRole("alert")).toContainText("нет сессии APGIC");
 await expect(form.getByRole("button",{name:"Проверить сохранение черновика"})).toHaveCount(0);
 expect(requests).toHaveLength(1);
});

test("lost draft response replays same per-owner request key, not a second draft",async({page})=>{
 const requests:string[]=[];let attempts=0;
 await mockAcademy(page,()=>{
  attempts++;
  if(attempts<=2)return {status:503,data:{code:"ACADEMY_UNAVAILABLE"}};
  return {status:201,data:draft};
 },outlineOk,()=>attempts>=3,requests);
 const form=page.locator("#academy-author");
 await form.getByRole("button",{name:"Создать курс",exact:true}).click();
 await expect(form.getByRole("alert")).toContainText("Не удалось подтвердить сохранение курса");
 await expect(form.getByRole("button",{name:"Создать курс",exact:true})).toBeDisabled();
 await form.getByRole("button",{name:"Проверить сохранение черновика"}).click();
 await expect(page.getByRole("heading",{name:"гипноз",exact:true})).toBeVisible();
 expect(requests).toHaveLength(3);
 expect(new Set(requests).size).toBe(1);
});

test("confirmed draft with failed editor load reopens by ID without another POST",async({page})=>{
 const requests:string[]=[];let outlineFailed=false;
 await mockAcademy(page,()=>({status:201,data:draft}),()=>{
  if(!outlineFailed){outlineFailed=true;return {status:503,data:{code:"ACADEMY_UNAVAILABLE"}};}
  return outlineOk();
 },()=>true,requests);
 const form=page.locator("#academy-author");
 await form.getByRole("button",{name:"Создать курс",exact:true}).click();
 await expect(form.getByRole("button",{name:"Открыть созданный курс"})).toBeVisible();
 await expect(form.getByRole("button",{name:"Создать курс",exact:true})).toBeDisabled();
 await expect(form).toContainText("«гипноз» создан");
 await form.getByRole("button",{name:"Открыть созданный курс"}).click();
 await expect(page.getByRole("heading",{name:"гипноз",exact:true})).toBeVisible();
 expect(requests).toHaveLength(1);
});


test("author can create a one-character course title accepted by the canonical API",async({page})=>{
 const requests:string[]=[];
 const shortDraft={...draft,title:"1"};
 await mockAcademy(page,()=>({status:201,data:shortDraft}),outlineOk,()=>true,requests,"1");
 const form=page.locator("#academy-author");
 const create=form.getByRole("button",{name:"Создать курс",exact:true});
 await expect(create).toBeEnabled();
 await create.click();
 await expect(page.getByRole("heading",{name:"1",exact:true})).toBeVisible();
 expect(requests).toHaveLength(1);
});
