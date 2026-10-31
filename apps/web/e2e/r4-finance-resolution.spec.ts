import { test, expect } from "@playwright/test";

test("R4 operator resolves a discrepancy through authenticated UI without client-issued grants", async ({page}) => {
  const id = "00000000-0000-0000-0000-000000000101";
  let resolved = false;
  let request: Record<string, unknown> = {};
  await page.route("**/v1/finance/operations/discrepancies?limit=100", route =>
    route.fulfill({status:200, contentType:"application/json", body:JSON.stringify({items:resolved?[]:[{
      discrepancy_id:id,kind:"STATEMENT_MISSING",currency:"RUB",
      expected_fee_minor:1,provider_fee_minor:0,
      expected_gross_minor:100,provider_gross_minor:0,
      created_at:"2026-10-19T10:00:00Z",provider_instance_id:"provider",
      settlement_instruction_id:"instruction",evidence_refs:["statement"]
    }]})}));
  await page.route("**/v1/finance/operations/discrepancies/*/resolutions", async route => {
    request = route.request().postDataJSON();
    resolved = true;
    await route.fulfill({status:201,contentType:"application/json",body:"{}"});
  });
  await page.goto("/finance/operations");
  await page.getByLabel("Расхождение",{exact:true}).selectOption(id);
  await page.getByLabel("Тип решения").selectOption("OPERATIONAL_EXCEPTION_ACCEPTED");
  await page.getByLabel("Обоснование").fill("Provider discrepancy reviewed with independent evidence");
  await page.getByLabel("Ссылки на доказательства, по одной на строку").fill("ticket/finance-123");
  await page.getByRole("button",{name:"Записать решение с серверной проверкой"}).click();
  await expect(page.getByText("Открытых расхождений нет.")).toBeVisible();
  expect(request.kind).toBe("OPERATIONAL_EXCEPTION_ACCEPTED");
  expect(request.evidence_refs).toEqual(["ticket/finance-123"]);
  expect(typeof request.idempotency_key).toBe("string");
  expect("operator_id" in request).toBe(false);
  expect("audit_ref" in request).toBe(false);
});
