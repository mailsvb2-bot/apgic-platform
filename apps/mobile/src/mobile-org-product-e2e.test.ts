import assert from "node:assert/strict";
import test from "node:test";

import {
  runNativeOrgProductE2E,
  type NativeOrgProductFetch,
} from "./mobile-org-product-e2e.ts";

test("native Organization/Product journey persists explicit ownership through archive", async () => {
  const organizationID = "00000000-0000-4000-8000-00000000a001";
  const directionID = "00000000-0000-4000-8000-00000000a002";
  const productID = "00000000-0000-4000-8000-00000000a003";
  const commercialOwnerRef = `organization/${organizationID}`;
  const authorRef = `identity/native-ios-author-${organizationID}`;
  const revenueRef = `identity/native-ios-beneficiary-${organizationID}`;
  const calls: Array<{url: string; method: string; body?: unknown}> = [];

  const request: NativeOrgProductFetch = async (url, options = {}) => {
    const method = options.method ?? "GET";
    const body = options.body ? JSON.parse(options.body) : undefined;
    calls.push({url, method, body});

    if (url.endsWith("/v1/organizations") && method === "POST") {
      return response(201, {id: organizationID, status: "ACTIVE", directions: []});
    }
    if (url.endsWith(`/v1/organizations/${organizationID}/directions`) && method === "POST") {
      return response(201, {
        id: organizationID,
        status: "ACTIVE",
        directions: [{
          id: directionID,
          organization_id: organizationID,
          direction_type: "SERVICE",
          status: "ACTIVE",
        }],
      });
    }
    if (url.endsWith(`/v1/organizations/${organizationID}/products`) && method === "POST") {
      return response(201, {
        id: productID,
        status: "DRAFT",
        owner_type: "ORGANIZATION",
        owner_id: organizationID,
        commercial_owner_ref: commercialOwnerRef,
        author_refs: [authorRef],
        revenue_beneficiary_ref: revenueRef,
        organization_direction_id: directionID,
      });
    }
    if (url.endsWith(`/products/${productID}/publish`) && method === "POST") {
      return response(200, {id: productID, status: "PUBLISHED"});
    }
    if (url.endsWith(`/directions/${directionID}/archive`) && method === "POST") {
      return response(200, {
        id: organizationID,
        directions: [{id: directionID, direction_type: "SERVICE", status: "ARCHIVED"}],
      });
    }
    if (url.endsWith(`/v1/organizations/${organizationID}`) && method === "GET") {
      return response(200, {
        id: organizationID,
        directions: [{id: directionID, direction_type: "SERVICE", status: "ARCHIVED"}],
      });
    }
    if (url.endsWith(`/v1/organizations/${organizationID}/products`) && method === "GET") {
      return response(200, {
        products: [{
          id: productID,
          status: "PUBLISHED",
          owner_type: "ORGANIZATION",
          owner_id: organizationID,
          commercial_owner_ref: commercialOwnerRef,
          author_refs: [authorRef],
          revenue_beneficiary_ref: revenueRef,
          organization_direction_id: directionID,
        }],
      });
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  };

  const result = await runNativeOrgProductE2E(
    {
      baseURL: "http://127.0.0.1:43113",
      sessionCookie: "__Host-apgic_session=signed",
      surface: "IOS",
    },
    request,
  );

  assert.deepEqual(result, {
    organizationCreated: true,
    genericDirectionCreated: true,
    productPublished: true,
    directionArchived: true,
    productPreservedAfterArchive: true,
    explicitOwnershipRolesPersisted: true,
    organizationID,
    directionID,
    productID,
  });
  assert.equal(calls.length, 7);
  assert.deepEqual(calls[2]?.body, {
    name: "Native IOS Product",
    direction_id: directionID,
    commercial_owner_ref: commercialOwnerRef,
    author_refs: [authorRef],
    revenue_beneficiary_ref: revenueRef,
  });
});

test("native Organization/Product journey fails closed when ownership readback drifts", async () => {
  const organizationID = "00000000-0000-4000-8000-00000000b001";
  const directionID = "00000000-0000-4000-8000-00000000b002";
  const productID = "00000000-0000-4000-8000-00000000b003";
  let step = 0;

  const request: NativeOrgProductFetch = async () => {
    step += 1;
    switch (step) {
      case 1:
        return response(201, {id: organizationID});
      case 2:
        return response(201, {
          directions: [{id: directionID, direction_type: "SERVICE", status: "ACTIVE"}],
        });
      case 3:
        return response(201, {
          id: productID,
          status: "DRAFT",
          owner_type: "ORGANIZATION",
          owner_id: organizationID,
          commercial_owner_ref: `organization/${organizationID}`,
          author_refs: [`identity/native-android-author-${organizationID}`],
          revenue_beneficiary_ref: `identity/native-android-beneficiary-${organizationID}`,
          organization_direction_id: directionID,
        });
      case 4:
        return response(200, {id: productID, status: "PUBLISHED"});
      case 5:
        return response(200, {
          directions: [{id: directionID, status: "ARCHIVED"}],
        });
      case 6:
        return response(200, {
          directions: [{id: directionID, status: "ARCHIVED"}],
        });
      case 7:
        return response(200, {
          products: [{
            id: productID,
            status: "PUBLISHED",
            owner_type: "ORGANIZATION",
            owner_id: organizationID,
            commercial_owner_ref: "organization/forged-owner",
            author_refs: [`identity/native-android-author-${organizationID}`],
            revenue_beneficiary_ref: `identity/native-android-beneficiary-${organizationID}`,
            organization_direction_id: directionID,
          }],
        });
      default:
        throw new Error("unexpected request");
    }
  };

  await assert.rejects(
    runNativeOrgProductE2E(
      {
        baseURL: "http://127.0.0.1:43113",
        sessionCookie: "__Host-apgic_session=signed",
        surface: "ANDROID",
      },
      request,
    ),
    /MOBILE_ORG_PRODUCT_PUBLISHED_PRODUCT_NOT_PRESERVED/,
  );
});

function response(status: number, payload: unknown) {
  return {
    status,
    async json() {
      return payload;
    },
  };
}
