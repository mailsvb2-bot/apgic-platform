#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

checks = {
    "apps/mobile/android/app/src/main/AndroidManifest.xml": [
        'android:autoVerify="true"',
        'android:scheme="https"',
        'android:host="apgic.ru"',
        'android:pathPrefix="/l/"',
    ],
    "apps/mobile/ios/APGIC/APGIC.entitlements": [
        "com.apple.developer.associated-domains",
        "applinks:apgic.ru",
    ],
    "apps/mobile/ios/APGIC.xcodeproj/project.pbxproj": [
        "CODE_SIGN_ENTITLEMENTS = APGIC/APGIC.entitlements;",
    ],
    "apps/mobile/ios/APGIC/AppDelegate.swift": [
        "RCTLinkingManager.application",
        "continue userActivity",
    ],
    "apps/web/src/mobile-app-link-association.ts": [
        "APGIC_ANDROID_APP_LINK_PACKAGE_NAME",
        "APGIC_ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS",
        "APGIC_IOS_APP_LINK_APP_IDS",
        'paths: ["/l/*"]',
    ],
    "apps/web/app/.well-known/assetlinks.json/route.ts": [
        "androidAssetLinksFromEnv",
        "APP_LINK_ASSOCIATION_NOT_CONFIGURED",
    ],
    "apps/web/app/.well-known/apple-app-site-association/route.ts": [
        "appleAppSiteAssociationFromEnv",
        "APP_LINK_ASSOCIATION_NOT_CONFIGURED",
    ],
    "backend/internal/mobile/deeplink_token.go": [
        "hmac.New(sha256.New",
        "MaxDeepLinkLifetime",
        "ResolveTrustedDeepLink",
    ],
    "contracts/openapi/apgic-v1.yaml": [
        "/v1/mobile/deep-links:",
        "/v1/mobile/deep-links/resolve:",
        "DeepLinkResolution:",
    ],
}

missing = []
for relative, needles in checks.items():
    path = ROOT / relative
    if not path.is_file():
        missing.append(f"missing file: {relative}")
        continue
    text = path.read_text(encoding="utf-8")
    for needle in needles:
        if needle not in text:
            missing.append(f"{relative}: missing {needle!r}")

association = (ROOT / "apps/web/src/mobile-app-link-association.ts").read_text(encoding="utf-8")
for forbidden in ("TEAMID.", "AA:BB:CC:", "example.com"):
    if forbidden in association:
        missing.append(f"association source contains placeholder production identity: {forbidden}")

if missing:
    print("MOBILE DEEP-LINK CONTRACT GUARD: FAIL")
    for item in missing:
        print(f"- {item}")
    raise SystemExit(1)

print("MOBILE DEEP-LINK CONTRACT GUARD: PASS")
