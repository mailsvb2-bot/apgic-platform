import { expect, test } from "@playwright/test";

type Organization = {
  id: string;
  name: string;
  status: string;
};

type ErrorEnvelope = {
  code: string;
  message_safe: string;
  policy_reason_codes?: string[];
};

test.skip(
  process.env.APGIC_AUTH001_WEB_E2E !== "1",
  "AUTH-001 PostgreSQL-backed browser proof runs only in the dedicated CI gate",
);

test("WEB browser enforces organization tenant isolation through the real API", async ({ browser }) => {
  const contextA = await browser.newContext();
  const contextB = await browser.newContext();
  const pageA = await contextA.newPage();
  const pageB = await contextB.newPage();

  await pageA.goto("/");
  await pageB.goto("/");

  const createOrganization = async (page: typeof pageA, name: string): Promise<Organization> =>
    page.evaluate(async (organizationName) => {
      const response = await fetch("/v1/organizations", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: organizationName }),
      });
      if (!response.ok) {
        throw new Error(`organization create failed: ${response.status} ${await response.text()}`);
      }
      return response.json() as Promise<Organization>;
    }, name);

  const orgA = await createOrganization(pageA, "AUTH001 WEB Organization A");
  const orgB = await createOrganization(pageB, "AUTH001 WEB TOP SECRET Organization B");

  expect(orgA.id).toBeTruthy();
  expect(orgB.id).toBeTruthy();
  expect(orgA.id).not.toBe(orgB.id);

  const own = await pageA.evaluate(async ({ organizationID }) => {
    const response = await fetch(`/v1/organizations/${organizationID}/private-profile`, {
      headers: {
        "X-Organization-Context": organizationID,
        "X-Correlation-Id": "auth001-web-own-allow",
      },
    });
    return {
      status: response.status,
      body: await response.text(),
    };
  }, { organizationID: orgA.id });

  expect(own.status).toBe(200);
  expect(own.body).toContain("AUTH001 WEB Organization A");

  const crossTenant = await pageA.evaluate(async ({ targetID, contextID }) => {
    const response = await fetch(`/v1/organizations/${targetID}/private-profile`, {
      headers: {
        "X-Organization-Context": contextID,
        "X-Correlation-Id": "auth001-web-cross-tenant-deny",
      },
    });
    return {
      status: response.status,
      body: await response.text(),
    };
  }, { targetID: orgB.id, contextID: orgA.id });

  expect(crossTenant.status).toBe(403);
  expect(crossTenant.body).not.toContain("AUTH001 WEB TOP SECRET Organization B");
  const crossEnvelope = JSON.parse(crossTenant.body) as ErrorEnvelope;
  expect(crossEnvelope.code).toBe("AUTH_CROSS_TENANT_DENY");
  expect(crossEnvelope.policy_reason_codes).toEqual(["AUTH_CROSS_TENANT_DENY"]);

  const forgedContext = await pageA.evaluate(async ({ targetID }) => {
    const response = await fetch(`/v1/organizations/${targetID}/private-profile`, {
      headers: {
        "X-Organization-Context": targetID,
        "X-Correlation-Id": "auth001-web-forged-context-deny",
      },
    });
    return {
      status: response.status,
      body: await response.text(),
    };
  }, { targetID: orgB.id });

  expect(forgedContext.status).toBe(403);
  expect(forgedContext.body).not.toContain("AUTH001 WEB TOP SECRET Organization B");
  const forgedEnvelope = JSON.parse(forgedContext.body) as ErrorEnvelope;
  expect(forgedEnvelope.code).toBe("AUTH_TENANT_CONTEXT_DENIED");
  expect(forgedEnvelope.policy_reason_codes).toEqual(["AUTH_TENANT_CONTEXT_DENIED"]);

  await contextA.close();
  await contextB.close();
});
