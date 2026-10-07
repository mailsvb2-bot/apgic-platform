declare const __DEV__: boolean;

export type NativeOrgProductSurface = "IOS" | "ANDROID";

export type NativeOrgProductE2EConfig = {
  baseURL: string;
  sessionCookie: string;
  surface: NativeOrgProductSurface;
};

export type NativeOrgProductE2EResult = {
  organizationCreated: true;
  genericDirectionCreated: true;
  productPublished: true;
  directionArchived: true;
  productPreservedAfterArchive: true;
  explicitOwnershipRolesPersisted: true;
  organizationID: string;
  directionID: string;
  productID: string;
};

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

export type NativeOrgProductFetch = (
  url: string,
  options?: FetchOptions,
) => Promise<FetchResponse>;

const defaultFetch: NativeOrgProductFetch = (url, options) => fetch(url, options);

function canonicalBaseURL(raw: string): string {
  const candidate = raw.trim().replace(/\/$/, "");
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error("MOBILE_ORG_PRODUCT_BASE_URL_INVALID");
  }
  const loopback =
    parsed.protocol === "http:" &&
    (parsed.hostname === "127.0.0.1" || parsed.hostname === "localhost");
  if (
    (parsed.protocol !== "https:" && !loopback) ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== "" ||
    (parsed.pathname !== "" && parsed.pathname !== "/")
  ) {
    throw new Error("MOBILE_ORG_PRODUCT_BASE_URL_INVALID");
  }
  return candidate;
}

function requireString(value: unknown, code: string): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error(code);
  }
  return value.trim();
}

function asObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("MOBILE_ORG_PRODUCT_RESPONSE_INVALID");
  }
  return value as Record<string, unknown>;
}

function asObjectArray(value: unknown, code: string): Record<string, unknown>[] {
  if (!Array.isArray(value) || !value.every((item) => item && typeof item === "object" && !Array.isArray(item))) {
    throw new Error(code);
  }
  return value as Record<string, unknown>[];
}

async function jsonRequest(
  request: NativeOrgProductFetch,
  sessionCookie: string,
  method: string,
  url: string,
  body?: unknown,
): Promise<{status: number; payload: Record<string, unknown>}> {
  const response = await request(url, {
    method,
    credentials: "include",
    headers: {
      Accept: "application/json",
      Cookie: sessionCookie,
      ...(body === undefined ? {} : {"Content-Type": "application/json"}),
    },
    ...(body === undefined ? {} : {body: JSON.stringify(body)}),
  });
  return {status: response.status, payload: asObject(await response.json())};
}

function requireStatus(actual: number, expected: number, code: string): void {
  if (actual !== expected) {
    throw new Error(code);
  }
}

export async function runNativeOrgProductE2E(
  config: NativeOrgProductE2EConfig,
  request: NativeOrgProductFetch = defaultFetch,
): Promise<NativeOrgProductE2EResult> {
  if (typeof __DEV__ !== "undefined" && !__DEV__) {
    throw new Error("MOBILE_ORG_PRODUCT_E2E_DEBUG_ONLY");
  }
  if (config.surface !== "IOS" && config.surface !== "ANDROID") {
    throw new Error("MOBILE_ORG_PRODUCT_SURFACE_INVALID");
  }

  const baseURL = canonicalBaseURL(config.baseURL);
  const sessionCookie = requireString(
    config.sessionCookie,
    "MOBILE_ORG_PRODUCT_SESSION_REQUIRED",
  );
  const suffix = config.surface.toLowerCase();

  const createdOrganization = await jsonRequest(
    request,
    sessionCookie,
    "POST",
    `${baseURL}/v1/organizations`,
    {name: `Native ${config.surface} Organization`},
  );
  requireStatus(
    createdOrganization.status,
    201,
    "MOBILE_ORG_PRODUCT_CREATE_ORGANIZATION_FAILED",
  );
  const organizationID = requireString(
    createdOrganization.payload.id,
    "MOBILE_ORG_PRODUCT_ORGANIZATION_ID_MISSING",
  );

  const createdDirection = await jsonRequest(
    request,
    sessionCookie,
    "POST",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/directions`,
    {name: `Native ${config.surface} Service`, direction_type: "SERVICE"},
  );
  requireStatus(
    createdDirection.status,
    201,
    "MOBILE_ORG_PRODUCT_CREATE_DIRECTION_FAILED",
  );
  const createdDirections = asObjectArray(
    createdDirection.payload.directions,
    "MOBILE_ORG_PRODUCT_DIRECTIONS_INVALID",
  );
  const activeDirection = createdDirections.find(
    (item) => item.direction_type === "SERVICE" && item.status === "ACTIVE",
  );
  if (!activeDirection) {
    throw new Error("MOBILE_ORG_PRODUCT_ACTIVE_DIRECTION_MISSING");
  }
  const directionID = requireString(
    activeDirection.id,
    "MOBILE_ORG_PRODUCT_DIRECTION_ID_MISSING",
  );

  const commercialOwnerRef = `organization/${organizationID}`;
  const authorRef = `identity/native-${suffix}-author-${organizationID}`;
  const revenueBeneficiaryRef = `identity/native-${suffix}-beneficiary-${organizationID}`;

  const createdProduct = await jsonRequest(
    request,
    sessionCookie,
    "POST",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/products`,
    {
      name: `Native ${config.surface} Product`,
      direction_id: directionID,
      commercial_owner_ref: commercialOwnerRef,
      author_refs: [authorRef],
      revenue_beneficiary_ref: revenueBeneficiaryRef,
    },
  );
  requireStatus(
    createdProduct.status,
    201,
    "MOBILE_ORG_PRODUCT_CREATE_PRODUCT_FAILED",
  );
  const productID = requireString(
    createdProduct.payload.id,
    "MOBILE_ORG_PRODUCT_PRODUCT_ID_MISSING",
  );

  if (
    createdProduct.payload.status !== "DRAFT" ||
    createdProduct.payload.owner_type !== "ORGANIZATION" ||
    createdProduct.payload.owner_id !== organizationID ||
    createdProduct.payload.commercial_owner_ref !== commercialOwnerRef ||
    JSON.stringify(createdProduct.payload.author_refs) !== JSON.stringify([authorRef]) ||
    createdProduct.payload.revenue_beneficiary_ref !== revenueBeneficiaryRef ||
    createdProduct.payload.organization_direction_id !== directionID
  ) {
    throw new Error("MOBILE_ORG_PRODUCT_EXPLICIT_ROLES_NOT_PERSISTED");
  }

  const published = await jsonRequest(
    request,
    sessionCookie,
    "POST",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/products/${encodeURIComponent(productID)}/publish`,
  );
  requireStatus(published.status, 200, "MOBILE_ORG_PRODUCT_PUBLISH_FAILED");
  if (published.payload.status !== "PUBLISHED") {
    throw new Error("MOBILE_ORG_PRODUCT_PUBLISH_STATE_INVALID");
  }

  const archived = await jsonRequest(
    request,
    sessionCookie,
    "POST",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/directions/${encodeURIComponent(directionID)}/archive`,
  );
  requireStatus(archived.status, 200, "MOBILE_ORG_PRODUCT_ARCHIVE_FAILED");
  const archivedDirections = asObjectArray(
    archived.payload.directions,
    "MOBILE_ORG_PRODUCT_ARCHIVE_DIRECTIONS_INVALID",
  );
  if (!archivedDirections.some((item) => item.id === directionID && item.status === "ARCHIVED")) {
    throw new Error("MOBILE_ORG_PRODUCT_ARCHIVE_STATE_INVALID");
  }

  const organizationReadback = await jsonRequest(
    request,
    sessionCookie,
    "GET",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}`,
  );
  requireStatus(
    organizationReadback.status,
    200,
    "MOBILE_ORG_PRODUCT_ORGANIZATION_READBACK_FAILED",
  );
  const readbackDirections = asObjectArray(
    organizationReadback.payload.directions,
    "MOBILE_ORG_PRODUCT_READBACK_DIRECTIONS_INVALID",
  );
  if (!readbackDirections.some((item) => item.id === directionID && item.status === "ARCHIVED")) {
    throw new Error("MOBILE_ORG_PRODUCT_DIRECTION_NOT_ARCHIVED_ON_READBACK");
  }

  const productsReadback = await jsonRequest(
    request,
    sessionCookie,
    "GET",
    `${baseURL}/v1/organizations/${encodeURIComponent(organizationID)}/products`,
  );
  requireStatus(
    productsReadback.status,
    200,
    "MOBILE_ORG_PRODUCT_PRODUCT_READBACK_FAILED",
  );
  const products = asObjectArray(
    productsReadback.payload.products,
    "MOBILE_ORG_PRODUCT_PRODUCTS_INVALID",
  );
  const product = products.find((item) => item.id === productID);
  if (
    !product ||
    product.status !== "PUBLISHED" ||
    product.owner_type !== "ORGANIZATION" ||
    product.owner_id !== organizationID ||
    product.commercial_owner_ref !== commercialOwnerRef ||
    JSON.stringify(product.author_refs) !== JSON.stringify([authorRef]) ||
    product.revenue_beneficiary_ref !== revenueBeneficiaryRef ||
    product.organization_direction_id !== directionID
  ) {
    throw new Error("MOBILE_ORG_PRODUCT_PUBLISHED_PRODUCT_NOT_PRESERVED");
  }

  return {
    organizationCreated: true,
    genericDirectionCreated: true,
    productPublished: true,
    directionArchived: true,
    productPreservedAfterArchive: true,
    explicitOwnershipRolesPersisted: true,
    organizationID,
    directionID,
    productID,
  };
}
