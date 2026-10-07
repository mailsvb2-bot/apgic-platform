import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test } from "@playwright/test";

type Direction = {
  id: string;
  organization_id: string;
  name: string;
  direction_type: string;
  status: "ACTIVE" | "ARCHIVED";
};

type Organization = {
  id: string;
  name: string;
  status: string;
  directions: Direction[];
};

type Product = {
  id: string;
  name: string;
  status: "DRAFT" | "PUBLISHED";
  owner_type: string;
  owner_id: string;
  commercial_owner_ref: string;
  author_refs: string[];
  revenue_beneficiary_ref: string;
  organization_direction_id: string;
};

test.skip(
  process.env.APGIC_ORG_PROD_WEB_E2E !== "1",
  "real Organization/Product PostgreSQL-backed browser proof runs only in the dedicated CI gate",
);

test("WEB organization product lifecycle persists through real API and PostgreSQL", async ({ page }) => {
  const candidateSHA = process.env.APGIC_CANDIDATE_SHA ?? "local";
  const suffix = candidateSHA.slice(0, 12);
  const organizationName = `R0 WEB Organization ${suffix}`;
  const directionName = `R0 Service ${suffix}`;
  const productName = `R0 Product ${suffix}`;

  await page.goto("/organization");
  await expect(page.getByRole("heading", { level: 1, name: /Управляйте организацией/ })).toBeVisible();

  await page.getByLabel("Название организации").fill(organizationName);
  await page.getByRole("button", { name: "Создать организацию" }).click();
  await expect(page.getByRole("heading", { name: organizationName })).toBeVisible();
  await expect(page.getByRole("status")).toContainText("активный владелец");

  const organizations = await page.evaluate(async () => {
    const response = await fetch("/v1/organizations", { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`organization list failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<{ organizations: Organization[] }>;
  });
  const organization = organizations.organizations.find((item) => item.name === organizationName);
  expect(organization?.id).toBeTruthy();
  if (!organization) throw new Error("created organization missing from canonical list");

  await page.getByLabel("Название направления").fill(directionName);
  await page.getByLabel("Тип направления").selectOption("SERVICE");
  await page.getByRole("button", { name: "Добавить направление" }).click();
  await expect(
    page.locator(".organization-directions li").filter({ hasText: directionName }).filter({ hasText: "SERVICE · ACTIVE" }),
  ).toHaveCount(1);

  const organizationAfterDirection = await page.evaluate(async (organizationID) => {
    const response = await fetch(`/v1/organizations/${organizationID}`, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`organization read failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<Organization>;
  }, organization.id);
  const direction = organizationAfterDirection.directions.find((item) => item.name === directionName);
  expect(direction?.status).toBe("ACTIVE");
  if (!direction) throw new Error("created direction missing from canonical organization");

  const commercialOwnerRef = `organization/${organization.id}`;
  const authorRef = `identity/web-author-${suffix}`;
  const revenueBeneficiaryRef = `identity/web-beneficiary-${suffix}`;

  await page.getByLabel("Название продукта").fill(productName);
  await page.getByLabel("Направление продукта").selectOption(direction.id);
  await page.getByLabel("Коммерческий владелец").fill(commercialOwnerRef);
  await page.getByLabel("Авторы").fill(authorRef);
  await page.getByLabel("Получатель выручки").fill(revenueBeneficiaryRef);
  await page.getByRole("button", { name: "Создать черновик продукта" }).click();

  const draftProducts = await page.evaluate(async (organizationID) => {
    const response = await fetch(`/v1/organizations/${organizationID}/products`, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`product list failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<{ products: Product[] }>;
  }, organization.id);
  const draft = draftProducts.products.find((item) => item.name === productName);
  expect(draft?.status).toBe("DRAFT");
  expect(draft?.owner_type).toBe("ORGANIZATION");
  expect(draft?.owner_id).toBe(organization.id);
  expect(draft?.commercial_owner_ref).toBe(commercialOwnerRef);
  expect(draft?.author_refs).toEqual([authorRef]);
  expect(draft?.revenue_beneficiary_ref).toBe(revenueBeneficiaryRef);
  expect(draft?.organization_direction_id).toBe(direction.id);
  if (!draft) throw new Error("created product missing from canonical product list");

  await page.getByRole("button", { name: "Опубликовать" }).click();
  await expect(page.getByRole("status")).toContainText("опубликован");

  const publishedProducts = await page.evaluate(async (organizationID) => {
    const response = await fetch(`/v1/organizations/${organizationID}/products`, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`published product list failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<{ products: Product[] }>;
  }, organization.id);
  const published = publishedProducts.products.find((item) => item.id === draft.id);
  expect(published?.status).toBe("PUBLISHED");
  expect(published?.commercial_owner_ref).toBe(commercialOwnerRef);
  expect(published?.author_refs).toEqual([authorRef]);
  expect(published?.revenue_beneficiary_ref).toBe(revenueBeneficiaryRef);

  await page.getByRole("button", { name: "Архивировать" }).click();
  await expect(page.getByText("SERVICE · ARCHIVED")).toBeVisible();
  await expect(page.getByRole("status")).toContainText("История сохранена");

  const archivedOrganization = await page.evaluate(async (organizationID) => {
    const response = await fetch(`/v1/organizations/${organizationID}`, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`archived organization read failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<Organization>;
  }, organization.id);
  expect(archivedOrganization.directions.find((item) => item.id === direction.id)?.status).toBe("ARCHIVED");

  const productsAfterArchive = await page.evaluate(async (organizationID) => {
    const response = await fetch(`/v1/organizations/${organizationID}/products`, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`post-archive product list failed: ${response.status} ${await response.text()}`);
    }
    return response.json() as Promise<{ products: Product[] }>;
  }, organization.id);
  expect(productsAfterArchive.products.find((item) => item.id === draft.id)?.status).toBe("PUBLISHED");

  await page.reload();
  await expect(page.getByRole("heading", { name: organizationName })).toBeVisible();
  await expect(page.getByText("SERVICE · ARCHIVED")).toBeVisible();
  const reloadedProduct = page.locator(".organization-products li").filter({ hasText: productName });
  await expect(reloadedProduct).toHaveCount(1);
  await expect(reloadedProduct).toContainText("PUBLISHED");
  await expect(reloadedProduct).toContainText(`Коммерческий владелец: ${commercialOwnerRef}`);
  await expect(reloadedProduct).toContainText(`Авторы: ${authorRef}`);
  await expect(reloadedProduct).toContainText(`Получатель выручки: ${revenueBeneficiaryRef}`);

  const evidenceDir = resolve(process.cwd(), "../../evidence");
  mkdirSync(evidenceDir, { recursive: true });
  writeFileSync(
    resolve(evidenceDir, "r0-org-product-web-e2e.json"),
    JSON.stringify(
      {
        schema_version: "r0-org-product-web-e2e-v1",
        evidence_type: "WEB_ORGANIZATION_PRODUCT_E2E",
        candidate_sha: candidateSHA,
        surface: "WEB",
        browser_through_next_proxy: true,
        postgres_backed_api: true,
        organization_created: true,
        generic_direction_created: true,
        explicit_product_ownership_roles_persisted: true,
        product_published: true,
        direction_archived_without_losing_product: true,
        persisted_after_browser_reload: true,
        organization_id: organization.id,
        direction_id: direction.id,
        product_id: draft.id,
        production_evidence: false,
      },
      null,
      2,
    ) + "\n",
    "utf8",
  );
});
