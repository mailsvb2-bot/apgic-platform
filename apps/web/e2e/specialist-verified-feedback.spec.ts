import {expect,test} from "@playwright/test";

const bookingID="22222222-2222-4222-8222-222222222222";
const booking={
 booking_id:bookingID,hold_id:"11111111-1111-4111-8111-111111111111",
 hold_expires_at:"2026-10-10T15:00:00Z",
 starts_at:"2026-10-10T17:00:00Z",ends_at:"2026-10-10T18:00:00Z",
 state:"COMPLETED"
};
const completed={
 booking_id:bookingID,provider_instance_id:"provider-verified",
 state:"COMPLETED",completion_evidence_ref:"provider-evidence/recorded-completion"
};

test("only completed canonical consultation exposes signed, pending-moderation rating",async({page})=>{
 await page.route(`**/v1/bookings/${bookingID}`,async r=>r.fulfill({json:booking}));
 await page.route(`**/v1/consultations/${bookingID}/result`,async r=>r.fulfill({json:completed}));
 let submitted=0;
 await page.route("**/v1/specialist-feedback",async r=>{
  submitted++;
  const payload=r.request().postDataJSON();
  expect(payload).toEqual({booking_id:bookingID,rating:5});
  expect(r.request().method()).toBe("POST");
  await r.fulfill({status:201,json:{id:"33333333-3333-4333-8333-333333333333",state:"PENDING",notice:"under review"}});
 });
 await page.goto(`/bookings/${bookingID}`);
 await page.getByRole("button",{name:"Проверить результат консультации"}).click();
 await expect(page.getByText("Консультация завершена по подтверждённому событию провайдера.")).toBeVisible();
 await page.getByLabel("Оцените проведённую консультацию").selectOption("5");
 await page.getByRole("button",{name:"Отправить подтверждённую оценку"}).click();
 await expect(page.getByText(/Она появится в рейтинге только после независимой проверки/)).toBeVisible();
 expect(submitted).toBe(1);
 await expect(page.getByRole("button",{name:"Отправить подтверждённую оценку"})).toBeDisabled();
});

test("noncompleted and forged provider outcomes never create feedback",async({page})=>{
 await page.route(`**/v1/bookings/${bookingID}`,async r=>r.fulfill({json:{...booking,state:"CONFIRMED"}}));
 await page.route(`**/v1/consultations/${bookingID}/result`,async r=>r.fulfill({
  json:{booking_id:bookingID,provider_instance_id:"provider-verified",state:"IN_PROGRESS"}
 }));
 let submissions=0;
 await page.route("**/v1/specialist-feedback",async r=>{submissions++;await r.fulfill({status:500,json:{}})});
 await page.goto(`/bookings/${bookingID}`);
 await page.getByRole("button",{name:"Проверить результат консультации"}).click();
 await expect(page.getByText("Завершение консультации пока не подтверждено.")).toBeVisible();
 await expect(page.getByLabel("Оцените проведённую консультацию")).toHaveCount(0);
 expect(submissions).toBe(0);
});

test("backend refusal cannot be interpreted as submitted approved review",async({page})=>{
 await page.route(`**/v1/bookings/${bookingID}`,async r=>r.fulfill({json:booking}));
 await page.route(`**/v1/consultations/${bookingID}/result`,async r=>r.fulfill({json:completed}));
 await page.route("**/v1/specialist-feedback",async r=>r.fulfill({
  status:409,json:{code:"SPECIALIST_FEEDBACK_NOT_ELIGIBLE",message_safe:"not eligible"}
 }));
 await page.goto(`/bookings/${bookingID}`);
 await page.getByRole("button",{name:"Проверить результат консультации"}).click();
 await page.getByLabel("Оцените проведённую консультацию").selectOption("4");
 await page.getByRole("button",{name:"Отправить подтверждённую оценку"}).click();
 await expect(page.getByRole("alert").getByText(/запись ещё не завершена/)).toBeVisible();
 await expect(page.getByText(/Она появится в рейтинге только после независимой проверки/)).toHaveCount(0);
});
