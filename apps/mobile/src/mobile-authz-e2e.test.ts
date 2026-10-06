import {
  runNativeTenantIsolationE2E,
  type NativeAuthzFetch,
} from "./mobile-authz-e2e.ts";

const config = {
  baseURL: "http://127.0.0.1:43113",
  sessionCookie: "__Host-apgic_session=e2e",
  ownOrganizationID: "00000000-0000-0000-0000-00000000a001",
  foreignOrganizationID: "00000000-0000-0000-0000-00000000b001",
  foreignPrivateMarker: "TOP SECRET AUTH001 FOREIGN",
  surface: "ANDROID" as const,
};

describe("native AUTH-001 tenant isolation E2E client", () => {
  it("proves own allow, two deterministic denies, no disclosure, and audit persistence", async () => {
    const calls: Array<{url: string; headers: Record<string, string>}> = [];
    const request: NativeAuthzFetch = async (url, options) => {
      const headers = options?.headers ?? {};
      calls.push({url, headers});
      if (url.endsWith("/private-profile")) {
        const context = headers["X-Organization-Context"];
        const correlation = headers["X-Correlation-Id"];
        if (url.includes(config.ownOrganizationID)) {
          return {
            status: 200,
            json: async () => ({
              id: config.ownOrganizationID,
              name: "AUTH001 OWN PRIVATE",
            }),
          };
        }
        if (context === config.ownOrganizationID) {
          expect(correlation).toBe("auth001-native-android-cross-tenant");
          return {
            status: 403,
            json: async () => ({
              code: "AUTH_CROSS_TENANT_DENY",
              message_safe: "Доступ к ресурсу запрещён.",
            }),
          };
        }
        expect(context).toBe(config.foreignOrganizationID);
        expect(correlation).toBe("auth001-native-android-forged-context");
        return {
          status: 403,
          json: async () => ({
            code: "AUTH_TENANT_CONTEXT_DENIED",
            message_safe: "Доступ к ресурсу запрещён.",
          }),
        };
      }

      const correlation = decodeURIComponent(url.split("/").pop() ?? "");
      return {
        status: 200,
        json: async () => ({
          correlation_id: correlation,
          reason: correlation.endsWith("cross-tenant")
            ? "AUTH_CROSS_TENANT_DENY"
            : "AUTH_TENANT_CONTEXT_DENIED",
          decision: "DENY",
        }),
      };
    };

    await expect(runNativeTenantIsolationE2E(config, request)).resolves.toEqual({
      sameTenantAllowed: true,
      crossTenantDenied: true,
      forgedContextDenied: true,
      privateDisclosureBlocked: true,
      crossTenantAuditPersisted: true,
      forgedContextAuditPersisted: true,
    });

    expect(calls.filter((call) => call.url.endsWith("/private-profile"))).toHaveLength(3);
    expect(calls.filter((call) => call.url.includes("/e2e/authz-audit/"))).toHaveLength(2);
    for (const call of calls.filter((value) => value.url.endsWith("/private-profile"))) {
      expect(call.headers.Cookie).toBe(config.sessionCookie);
    }
  });

  it("fails closed if a denial leaks the foreign private marker", async () => {
    const request: NativeAuthzFetch = async (url, options) => {
      const headers = options?.headers ?? {};
      if (url.includes(config.ownOrganizationID)) {
        return {
          status: 200,
          json: async () => ({id: config.ownOrganizationID, name: "OWN"}),
        };
      }
      if (url.endsWith("/private-profile")) {
        return {
          status: 403,
          json: async () => ({
            code:
              headers["X-Organization-Context"] === config.ownOrganizationID
                ? "AUTH_CROSS_TENANT_DENY"
                : "AUTH_TENANT_CONTEXT_DENIED",
            leaked: config.foreignPrivateMarker,
          }),
        };
      }
      return {
        status: 200,
        json: async () => ({decision: "DENY"}),
      };
    };

    await expect(runNativeTenantIsolationE2E(config, request)).rejects.toThrow(
      "MOBILE_AUTHZ_CROSS_TENANT_DENY_FAILED",
    );
  });
});
