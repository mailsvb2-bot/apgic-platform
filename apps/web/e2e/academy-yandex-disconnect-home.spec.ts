import {expect,test} from "@playwright/test";

// The author can revoke their Disk connection from the home screen even
// without a course or a video lesson. This browser fixture does not claim a
// live OAuth token: PostgreSQL and provider admission are separately gated.
test("author confirms Yandex Disk disconnect without needing a draft",async({page})=>{
 let disconnects=0;
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request();
  const path=new URL(request.url()).pathname;
  const method=request.method();
  let status=200;
  let body:unknown={};
  if(method==="POST"&&path==="/v1/academy/author/cloud/yandex/disconnect"){
   expect(request.postDataJSON()).toEqual({confirm:"DISCONNECT_YANDEX_DISK"});
   disconnects++;
   body={disconnected:true};
  }else if(method==="GET"&&path==="/v1/academy/courses")body={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-courses")body={courses:[]};
  else if(method==="GET"&&path==="/v1/academy/my-enrollments")body={enrollments:[]};
  else if(method==="GET"&&path==="/v1/academy/review/attempts")body={attempts:[]};
  else if(method==="GET"&&path==="/v1/academy/author/report")body={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
  else if(method==="GET"&&path==="/v1/academy/author/cohorts")body={cohorts:[]};
  else if(method==="GET"&&path==="/v1/academy/me/course-access")body={course_ids:[]};
  else if(method==="GET"&&path==="/v1/academy/me/certificates")body={certificates:[]};
  else{status=501;body={code:"UNMOCKED_ACADEMY_ROUTE",method,path}}
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(body)});
 });
 await page.goto("/academy");
 await expect(page.getByRole("heading",{name:"Кабинет автора"})).toBeVisible();
 const revoke=page.getByRole("button",{name:"Отключить Яндекс Диск"});
 await expect(revoke).toBeEnabled();
 page.once("dialog",dialog=>dialog.dismiss());
 await revoke.click();
 expect(disconnects).toBe(0);
 page.once("dialog",dialog=>dialog.accept());
 await revoke.click();
 await expect(page.getByText("Яндекс Диск отключён.",{exact:false})).toBeVisible();
 expect(disconnects).toBe(1);
});
