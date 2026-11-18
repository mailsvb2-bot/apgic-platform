import {expect,test} from "@playwright/test";

const courseID="11111111-1111-4111-8111-111111111111";
const versionID="22222222-2222-4222-8222-222222222222";

test("author saves paid price; learner cannot fake bank payment; free course can enroll",async({page})=>{
 let course={
  id:courseID,version_id:versionID,version_no:1,product_id:courseID,
  title:"Авторский курс",description:"Практика",state:"PUBLISHED",
  access_mode:"UNLISTED",price_minor:0,currency:"RUB"
 };
 const mutations:{path:string;body:unknown}[]=[];
 await page.route("**/v1/academy/**",async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;
  const method=request.method(),body=method==="POST"?request.postDataJSON():undefined;
  let data:unknown={},status=200;
  if(method==="GET"){
   if(path==="/v1/academy/courses"||path==="/v1/academy/my-courses")data={courses:[course]};
   else if(path==="/v1/academy/my-enrollments")data={enrollments:[]};
   else if(path==="/v1/academy/me/course-access")data={course_ids:[]};
   else if(path==="/v1/academy/me/certificates")data={certificates:[]};
   else if(path==="/v1/academy/review/attempts")data={attempts:[]};
   else if(path==="/v1/academy/author/report")data={courses:[],gross_captures:[],revenue_state:"UNAVAILABLE"};
   else if(path==="/v1/academy/author/cohorts")data={cohorts:[]};
   else if(path===`/v1/academy/courses/${courseID}/versions/${versionID}/outline`)data={state:"PUBLISHED",modules:[],lessons:[]};
   else if(path===`/v1/academy/courses/${courseID}/versions/${versionID}/rights/original`)
    data={confirmed:false,provenance_ref:""};
   else data={cohorts:[],lessons:[],attempts:[]};
  }else if(method==="POST"){
   mutations.push({path,body});
   if(path===`/v1/academy/courses/${courseID}/offer`){
    const offer=body as {access_mode:"FREE"|"PAID";price_minor:number};
    course={...course,access_mode:offer.access_mode,price_minor:offer.price_minor};
    data={course_id:courseID,...offer,currency:"RUB",revision:1};
   }else if(path===`/v1/academy/courses/${courseID}/checkout`){
    status=503;data={code:"ACADEMY_EXTERNAL_CHECKOUT_NOT_CONFIGURED",message_safe:"Оплата через Т-Банк пока не подключена."};
   }else if(path===`/v1/academy/courses/${courseID}/enroll`){
    status=201;data={id:"33333333-3333-4333-8333-333333333333",version_id:versionID,
     state:"ACTIVE",access_state:"ACTIVE"};
   }else{status=501;data={code:"UNMOCKED_MUTATION"}}
  }
  await route.fulfill({status,contentType:"application/json",body:JSON.stringify(data)});
 });
 await page.goto("/academy?tab=courses");
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 const offer=page.getByRole("region",{name:"Доступ и стоимость курса"});
 await expect(offer.getByText(/Доступ не открыт/)).toBeVisible();
 await offer.getByLabel(/Платный/).check();
 await offer.getByRole("spinbutton",{name:"Цена курса в рублях"}).fill("2499");
 await offer.getByRole("button",{name:"Сохранить"}).click();
 await expect(offer.locator(".academy-offer-current strong")).toContainText(/Платный.*2.499/);
 await expect(offer.getByRole("status")).toContainText("Сохранено: 2");
 const saveColor=await offer.getByRole("button",{name:"Сохранить"}).evaluate(el=>getComputedStyle(el).backgroundColor);
 expect(saveColor).toBe("rgb(23, 99, 237)");
 expect(mutations.find(x=>x.path.endsWith("/offer"))?.body).toEqual({access_mode:"PAID",price_minor:249900});
 await page.getByRole("button",{name:"Каталог",exact:true}).click();
 const card=page.getByRole("article").filter({hasText:"Авторский курс"});
 await expect(card.getByText(/2.499.*₽/)).toBeVisible();
 await card.getByRole("button",{name:"Купить курс"}).click();
 await expect(card.getByRole("alert")).toContainText("Т-Банк пока не подключена");
 expect(mutations.filter(x=>x.path.endsWith("/enroll"))).toHaveLength(0);
 await page.getByRole("button",{name:"Мои курсы",exact:true}).click();
 await page.getByRole("button",{name:"Открыть редактор"}).click();
 await offer.getByLabel(/Открытый/).check();
 await offer.getByRole("button",{name:"Сохранить"}).click();
 await expect(offer.getByRole("status")).toContainText("Сохранено: курс доступен бесплатно.");
 await page.getByRole("button",{name:"Каталог",exact:true}).click();
 await expect(card.getByText(/Бесплатно/)).toBeVisible();
 await card.getByRole("button",{name:"Начать бесплатно"}).click();
 await expect.poll(()=>mutations.filter(x=>x.path.endsWith("/enroll")).length).toBe(1);
 expect(mutations.filter(x=>x.path.endsWith("/checkout"))).toHaveLength(1);
});
