import { test, expect } from "@playwright/test";

test("R4 operator TOTP accepts six digits and sends them to the verified endpoint", async ({ page }) => {
  let received = "";
  await page.route("**/v1/finance/operations/discrepancies?limit=100", route =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({items:[{
      discrepancy_id:"00000000-0000-0000-0000-000000000101",
      kind:"STATEMENT_MISSING", currency:"RUB", expected_fee_minor:1,
      provider_fee_minor:0, expected_gross_minor:100, provider_gross_minor:0,
      created_at:"2026-10-19T10:00:00Z", provider_instance_id:"provider",
      settlement_instruction_id:"instruction", evidence_refs:["statement"]
    }]}) }),
  );
  await page.route("**/v1/finance/mfa/verify", route => {
    received = route.request().postDataJSON().code;
    return route.fulfill({status:204});
  });
  await page.goto("/finance/operations");
  await page.getByLabel("Расхождение", {exact:true}).selectOption("00000000-0000-0000-0000-000000000101");
  const input = page.getByLabel("Одноразовый код");
  await input.fill("12ab3456");
  await expect(input).toHaveValue("123456");
  await page.getByRole("button", {name:"Подтвердить второй фактор"}).click();
  await expect(page.getByText("Код принят.", {exact:false})).toBeVisible();
  expect(received).toBe("123456");
});
