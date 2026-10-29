import {expect,test} from "@playwright/test";

const booking="123e4567-e89b-42d3-a456-426614174001";
const identity="123e4567-e89b-42d3-a456-426614174000";
const makeView=(more=false)=>({
  identity_id:identity,as_of:"2026-10-12T10:00:00Z",
  preferences:{display_name:"Анна",locale:"ru",time_zone:"Europe/Moscow"},
  upcoming:[{booking_id:booking,state:"CONFIRMED",starts_at:"2026-10-15T14:00:00Z",ends_at:"2026-10-15T15:00:00Z",specialist_name:"Ирина Тестовая",specialist_profession:"PSYCHOLOGIST"}],
  history:more?[{booking_id:"123e4567-e89b-42d3-a456-426614174002",state:"COMPLETED",starts_at:"2026-10-01T12:00:00Z",ends_at:"2026-10-01T13:00:00Z",specialist_name:"Ирина Тестовая",specialist_profession:"PSYCHOLOGIST",consultation_state:"COMPLETED"}]:[],
  requests:[{id:"123e4567-e89b-42d3-a456-426614174003",state:"CONFIRMED",topics:["sleep"],created_at:"2026-10-09T12:00:00Z"}],
  ...(!more?{next_history_cursor:"test-cursor"}:{}),
});
test("personal workspace loads owner snapshot, saves preferences and advances history",async({page})=>{
  let saved=false;
  await page.route("**/v1/client/personal/preferences",route=>{
    saved=true;
    return route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({display_name:"Аня",locale:"ru",time_zone:"Europe/Moscow"})});
  });
  await page.route("**/v1/client/personal**",route=>{
    if(new URL(route.request().url()).pathname!=="/v1/client/personal")return route.fallback();
    const more=new URL(route.request().url()).searchParams.has("history_cursor");
    return route.fulfill({status:200,contentType:"application/json",headers:{"cache-control":"no-store"},body:JSON.stringify(makeView(more))});
  });
  await page.goto("/personal");
  await expect(page.getByRole("heading",{name:"Мой кабинет"})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("button",{name:"Показать более ранние записи"}).click();
  await expect(page.getByText("Завершена").first()).toBeVisible();
  await page.getByRole("textbox",{name:"Как к вам обращаться"}).fill("Аня");
  await page.getByRole("button",{name:"Сохранить настройки"}).click();
  await expect.poll(()=>saved).toBe(true);
});
test("deletion request status remains accessible when personal history API is unavailable",async({page})=>{
  await page.route("**/v1/client/personal",r=>r.fulfill({
    status:503,contentType:"application/json",body:'{"code":"PERSONAL_UNAVAILABLE"}',
  }));
  await page.route("**/v1/client/account-deletion-requests",r=>r.fulfill({
    status:200,contentType:"application/json",body:JSON.stringify({
      id:"123e4567-e89b-42d3-a456-426614174014",
      state:"REQUESTED",source:"WEB",
      created_at:"2026-10-27T12:00:00Z",updated_at:"2026-10-27T12:00:00Z",
      notice:"Стирание данных ещё не выполнено.",idempotent:false,pending_providers:0,
    }),
  }));
  await page.goto("/personal");
  await expect(page.getByRole("alert").filter({hasText:"Сейчас не удалось получить данные"})).toBeVisible();
  await expect(page.getByRole("heading",{name:"Удаление учётной записи"})).toBeVisible();
  await page.getByRole("button",{name:"Проверить статус запроса"}).click();
  await expect(page.getByRole("status").filter({hasText:"Запрос получен. Необходимо подтвердить личность."})).toBeVisible();
  await expect(page.getByRole("heading",{name:"Предстоящие записи"})).toHaveCount(0);
});

test("personal workspace fails closed without signed session",async({page})=>{
  await page.route("**/v1/client/personal",r=>r.fulfill({status:401,body:'{"code":"CLIENT_SESSION_REQUIRED"}'}));
  await page.goto("/personal");
  await expect(page.getByRole("alert").filter({hasText:"Нет подтверждённой сессии"})).toContainText("Нет подтверждённой сессии");
  await expect(page.getByRole("heading",{name:"Предстоящие записи"})).toHaveCount(0);
});

test("anonymous acquisition stays stable and exposes no untrusted account entry",async({page})=>{
  await page.route("**/v1/client/personal",r=>r.fulfill({status:401,body:'{"code":"CLIENT_SESSION_REQUIRED"}'}));
  await page.goto("/");
  await expect(page.getByRole("heading",{level:1})).toBeVisible();
  await expect(page.getByRole("link",{name:"Мой кабинет"})).toHaveCount(0);
});
test("signed client can enter personal workspace from the same public landing page",async({page})=>{
  await page.route("**/v1/client/personal",r=>r.fulfill({
    status:200,contentType:"application/json",body:JSON.stringify(makeView()),
  }));
  await page.goto("/");
  await page.getByRole("link",{name:"Мой кабинет"}).click();
  await expect(page).toHaveURL(/\/personal$/);
  await expect(page.getByRole("heading",{name:"Предстоящие записи"})).toBeVisible();
});

test("expired signed session during history paging clears private records and requires sign-in",async({page})=>{
  await page.route("**/v1/client/personal**",route=>{
    if(new URL(route.request().url()).pathname!=="/v1/client/personal")return route.fallback();
    const cursor=new URL(route.request().url()).searchParams.has("history_cursor");
    return route.fulfill(cursor?
      {status:401,contentType:"application/json",body:'{"code":"CLIENT_SESSION_EXPIRED"}'}:
      {status:200,contentType:"application/json",body:JSON.stringify(makeView())});
  });
  await page.goto("/personal");
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("button",{name:"Показать более ранние записи"}).click();
  await expect(page.getByRole("alert").filter({hasText:"Нет подтверждённой сессии"})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая")).toHaveCount(0);
  await expect(page.getByRole("heading",{name:"Предстоящие записи"})).toHaveCount(0);
});

test("expired signed session during personal preferences write clears old identity",async({page})=>{
  await page.route("**/v1/client/personal/preferences",r=>r.fulfill({
    status:403,contentType:"application/json",body:'{"code":"CLIENT_SESSION_EXPIRED"}',
  }));
  await page.route("**/v1/client/personal",r=>r.fulfill({
    status:200,contentType:"application/json",body:JSON.stringify(makeView()),
  }));
  await page.goto("/personal");
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("textbox",{name:"Как к вам обращаться"}).fill("Аня");
  await page.getByRole("button",{name:"Сохранить настройки"}).click();
  await expect(page.getByRole("alert").filter({hasText:"Нет подтверждённой сессии"})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая")).toHaveCount(0);
  await expect(page.getByRole("heading",{name:"Персональные настройки"})).toHaveCount(0);
});

test("expired signed session during a check-in also erases the old client history",async({page})=>{
  await page.route("**/v1/client/personal/check-ins",r=>r.fulfill({
    status:401,contentType:"application/json",body:'{"code":"CLIENT_SESSION_EXPIRED"}',
  }));
  await page.route("**/v1/client/personal**",r=>{
    if(new URL(r.request().url()).pathname!=="/v1/client/personal")return r.fallback();
    return r.fulfill({status:200,contentType:"application/json",body:JSON.stringify(makeView(true))});
  });
  await page.goto("/personal");
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("button",{name:"Сохранить оценку"}).click();
  await expect(page.getByRole("alert").filter({hasText:"Нет подтверждённой сессии"})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая")).toHaveCount(0);
});

test("personal workspace submits persistent deletion request and never claims completed erasure",async({page})=>{
  let requestRecorded=false;
  await page.route("**/v1/client/personal",route=>route.fulfill({
    status:200,contentType:"application/json",body:JSON.stringify(makeView()),
  }));
  await page.route("**/v1/client/account-deletion-requests",route=>{
    if(route.request().method()==="GET"){
      return route.fulfill(requestRecorded
        ?{status:200,contentType:"application/json",body:JSON.stringify({
          id:"123e4567-e89b-42d3-a456-426614174014",state:"REQUESTED",source:"WEB",
          created_at:"2026-10-27T12:00:00Z",updated_at:"2026-10-27T12:00:00Z",
          notice:"Стирание данных ещё не выполнено.",idempotent:false,
        })}
        :{status:404,contentType:"application/json",body:'{"code":"DELETION_REQUEST_NOT_FOUND"}'});
    }
    const body=route.request().postDataJSON();
    expect(body).toEqual({source:"WEB"});
    requestRecorded=true;
    return route.fulfill({status:202,contentType:"application/json",body:JSON.stringify({
      id:"123e4567-e89b-42d3-a456-426614174014",state:"REQUESTED",source:"WEB",
      created_at:"2026-10-27T12:00:00Z",updated_at:"2026-10-27T12:00:00Z",
      notice:"Стирание данных ещё не выполнено.",idempotent:false,
    })});
  });
  await page.goto("/personal");
  await expect(page.getByRole("heading",{name:"Удаление учётной записи"})).toBeVisible();
  const submit=page.getByRole("button",{name:"Подать запрос на удаление аккаунта"});
  await expect(submit).toBeDisabled();
  await page.getByRole("checkbox",{name:/понимаю/i}).check();
  await submit.click();
  await expect(page.getByRole("status").filter({hasText:"Запрос получен. Необходимо подтвердить личность."})).toBeVisible();
  await expect(page.getByText("Стирание данных ещё не выполнено.")).toBeVisible();
  await page.reload();
  await page.getByRole("button",{name:"Проверить статус запроса"}).click();
  await expect(page.getByRole("status").filter({hasText:"Запрос получен. Необходимо подтвердить личность."})).toBeVisible();
  await expect(page.getByRole("button",{name:"Подать запрос на удаление аккаунта"})).toHaveCount(0);
});

test("R5 verified deletion confirmation follows real user action, never silently hides cabinet",async({page})=>{
  const deletion=(state:string)=>({
    id:"123e4567-e89b-42d3-a456-426614174014",
    state,source:"WEB",created_at:"2026-10-27T12:00:00Z",
    updated_at:"2026-10-27T12:00:00Z",
    notice:state==="REQUESTED"?"Удаление ещё не выполнено.":"Подтверждение принято. Удаление ещё не выполнено.",
    idempotent:false,
  });
  let confirmed=false;
  await page.route("**/v1/client/personal",r=>r.fulfill({
    status:200,contentType:"application/json",body:JSON.stringify(makeView()),
  }));
  await page.route("**/v1/client/account-deletion-requests**",route=>{
    const req=route.request();
    const pathname=new URL(req.url()).pathname;
    if(pathname.endsWith("/factor")){
      return route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
        enrolled:true,ready_at:"2026-10-26T12:00:00Z",
      })});
    }
    if(pathname.endsWith("/factor/verify")){
      expect(req.headers()["x-apgic-deletion-factor"]).toBe("verify-v1");
      expect(req.postDataJSON()).toEqual({code:"123456"});
      return route.fulfill({status:204,body:""});
    }
    if(pathname.endsWith("/reconfirm")){
      expect(req.headers()["x-apgic-deletion-confirmation"]).toBe("verified-step-up-v1");
      expect(req.postDataJSON()).toEqual({confirm:true});
      confirmed=true;
      return route.fulfill({status:200,contentType:"application/json",body:JSON.stringify(deletion("IDENTITY_RECONFIRMED"))});
    }
    if(pathname.endsWith("/account-deletion-requests")&&req.method()==="GET"){
      return route.fulfill({status:200,contentType:"application/json",
        body:JSON.stringify(deletion(confirmed?"IDENTITY_RECONFIRMED":"REQUESTED"))});
    }
    return route.fulfill({status:404});
  });
  await page.goto("/personal");
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await page.getByRole("button",{name:"Проверить статус запроса"}).click();
  await expect(page.getByRole("status").filter({hasText:"Запрос получен. Необходимо подтвердить личность."})).toBeVisible();
  await page.getByRole("button",{name:"Проверить готовность подтверждения"}).click();
  await page.getByRole("textbox",{name:"Код из приложения"}).fill("123456");
  await page.getByRole("button",{name:"Проверить код"}).click();
  await page.getByRole("button",{name:"Подтвердить личность для удаления"}).click();
  await expect(page.getByRole("status").filter({hasText:"Личность подтверждена. Ожидается проверка правил хранения."})).toBeVisible();
  await expect(page.getByText("Ирина Тестовая").first()).toBeVisible();
  await expect(page.getByText("Подтверждение принято. Удаление ещё не выполнено.")).toBeVisible();
});
