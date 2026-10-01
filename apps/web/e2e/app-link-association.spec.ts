import { expect, test } from "@playwright/test";

import { GET as appleAssociationGET } from "../app/.well-known/apple-app-site-association/route";
import { GET as androidAssociationGET } from "../app/.well-known/assetlinks.json/route";
import {
  androidAssetLinksFromEnv,
  appleAppSiteAssociationFromEnv,
} from "../src/mobile-app-link-association";

const envNames = [
  "APGIC_ANDROID_APP_LINK_PACKAGE_NAME",
  "APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS",
  "APGIC_IOS_APP_LINK_APP_IDS",
] as const;

const originalEnv = Object.fromEntries(
  envNames.map((name) => [name, process.env[name]]),
) as Record<(typeof envNames)[number], string | undefined>;

function clearAssociationEnv() {
  for (const name of envNames) {
    delete process.env[name];
  }
}

function configureAssociationEnv() {
  process.env.APGIC_ANDROID_APP_LINK_PACKAGE_NAME = "com.apgic.ci";
  process.env.APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS = Array.from(
    { length: 32 },
    () => "AA",
  ).join(":");
  process.env.APGIC_IOS_APP_LINK_APP_IDS = "ABCDEFGHIJ.com.apgic.ci";
}

test.afterEach(() => {
  for (const name of envNames) {
    const value = originalEnv[name];
    if (value === undefined) {
      delete process.env[name];
    } else {
      process.env[name] = value;
    }
  }
});

test("verified-domain association routes fail closed without release identity", async () => {
  clearAssociationEnv();

  expect(() => androidAssetLinksFromEnv()).toThrow(
    "APGIC_ANDROID_APP_LINK_PACKAGE_NAME_NOT_CONFIGURED",
  );
  expect(() => appleAppSiteAssociationFromEnv()).toThrow(
    "APGIC_IOS_APP_LINK_APP_IDS_NOT_CONFIGURED",
  );

  const android = androidAssociationGET();
  const apple = appleAssociationGET();
  expect(android.status).toBe(503);
  expect(apple.status).toBe(503);
  expect(android.headers.get("cache-control")).toBe("no-store");
  expect(apple.headers.get("cache-control")).toBe("no-store");
});

test("verified-domain association payloads bind exact app identities and /l path", async () => {
  configureAssociationEnv();
  const fingerprint = process.env.APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS!;

  expect(androidAssetLinksFromEnv()).toEqual([
    {
      relation: ["delegate_permission/common.handle_all_urls"],
      target: {
        namespace: "android_app",
        package_name: "com.apgic.ci",
        sha256_cert_fingerprints: [fingerprint],
      },
    },
  ]);
  expect(appleAppSiteAssociationFromEnv()).toEqual({
    applinks: {
      apps: [],
      details: [
        {
          appID: "ABCDEFGHIJ.com.apgic.ci",
          paths: ["/l/*"],
        },
      ],
    },
  });

  const android = androidAssociationGET();
  const apple = appleAssociationGET();
  expect(android.status).toBe(200);
  expect(apple.status).toBe(200);
  expect(await android.json()).toEqual(androidAssetLinksFromEnv());
  expect(await apple.json()).toEqual(appleAppSiteAssociationFromEnv());
});

test("verified-domain association rejects malformed release identity", () => {
  configureAssociationEnv();
  process.env.APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS = "AA:BB";
  expect(() => androidAssetLinksFromEnv()).toThrow(
    "APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS_INVALID",
  );

  configureAssociationEnv();
  process.env.APGIC_IOS_APP_LINK_APP_IDS = "TEAMID.com.apgic.ci";
  expect(() => appleAppSiteAssociationFromEnv()).toThrow(
    "APGIC_IOS_APP_LINK_APP_IDS_INVALID",
  );
});
