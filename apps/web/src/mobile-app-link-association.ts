const ANDROID_RELATION = "delegate_permission/common.handle_all_urls" as const;

function requiredEnv(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name}_NOT_CONFIGURED`);
  }
  return value;
}

function csv(name: string): string[] {
  const values = requiredEnv(name)
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
  if (values.length === 0) {
    throw new Error(`${name}_NOT_CONFIGURED`);
  }
  return [...new Set(values)];
}

export function androidAssetLinksFromEnv() {
  const packageName = requiredEnv("APGIC_ANDROID_APP_LINK_PACKAGE_NAME");
  if (!/^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$/.test(packageName)) {
    throw new Error("APGIC_ANDROID_APP_LINK_PACKAGE_NAME_INVALID");
  }
  const fingerprints = csv("APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS").map(
    (value) => {
      if (!/^(?:[A-Fa-f0-9]{2}:){31}[A-Fa-f0-9]{2}$/.test(value)) {
        throw new Error("APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS_INVALID");
      }
      return value.toUpperCase();
    },
  );
  return [
    {
      relation: [ANDROID_RELATION],
      target: {
        namespace: "android_app",
        package_name: packageName,
        sha256_cert_fingerprints: fingerprints,
      },
    },
  ];
}

export function appleAppSiteAssociationFromEnv() {
  const appIDs = csv("APGIC_IOS_APP_LINK_APP_IDS");
  for (const appID of appIDs) {
    if (!/^[A-Z0-9]{10}\.[A-Za-z0-9][A-Za-z0-9.-]+$/.test(appID)) {
      throw new Error("APGIC_IOS_APP_LINK_APP_IDS_INVALID");
    }
  }
  return {
    applinks: {
      apps: [],
      details: appIDs.map((appID) => ({
        appID,
        paths: ["/l/*"],
      })),
    },
  };
}
