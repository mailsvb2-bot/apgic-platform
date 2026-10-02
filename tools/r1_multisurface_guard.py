#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SHARED = ROOT / "packages/contracts/src/r1-mobile.ts"
SCHEMA = ROOT / "contracts/jsonschema/r1-mobile-cross-surface-v1.schema.json"
WEB = ROOT / "apps/web/src/r1-cross-surface.ts"
MOBILE = ROOT / "apps/mobile/src/r1-cross-surface.ts"
WEB_ANALYTICS = ROOT / "apps/web/src/analytics-client.ts"
MOBILE_ANALYTICS = ROOT / "apps/mobile/src/analytics-client.ts"
ANALYTICS_E2E = ROOT / "tools/r1_analytics_parity_e2e.mjs"

CANONICAL_EVENTS = {
    "specialist_discovery_viewed",
    "workspace_switched",
    "account_deletion_requested",
}
FORBIDDEN_CONSUMER_DECLARATIONS = {
    "CanonicalAnalyticsEventV1",
    "DeleteAccountState",
    "AuthorizedWorkspaceV1",
    "DeepLinkResolutionV1",
}


def main() -> None:
    errors: list[str] = []
    for path in (SHARED, SCHEMA, WEB, MOBILE, WEB_ANALYTICS, MOBILE_ANALYTICS, ANALYTICS_E2E):
        if not path.is_file():
            errors.append(f"missing R1 multi-surface contract file: {path.relative_to(ROOT)}")

    if errors:
        report(errors)

    shared = SHARED.read_text(encoding="utf-8")
    web = WEB.read_text(encoding="utf-8")
    mobile = MOBILE.read_text(encoding="utf-8")
    web_analytics = WEB_ANALYTICS.read_text(encoding="utf-8")
    mobile_analytics = MOBILE_ANALYTICS.read_text(encoding="utf-8")
    analytics_e2e = ANALYTICS_E2E.read_text(encoding="utf-8")
    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    expected_import = "../../../packages/contracts/src/r1-mobile"
    for label, content in (("WEB", web), ("MOBILE", mobile)):
        if expected_import not in content:
            errors.append(f"{label}: does not consume the shared R1 mobile contract")
        for name in FORBIDDEN_CONSUMER_DECLARATIONS:
            if re.search(rf"\b(?:type|interface)\s+{re.escape(name)}\b", content):
                errors.append(f"{label}: redeclares canonical contract {name}")

    defs = schema.get("$defs") or {}
    surfaces = set((defs.get("ClientSurface") or {}).get("enum") or [])
    if surfaces != {"WEB", "IOS", "ANDROID"}:
        errors.append(f"ClientSurface schema mismatch: {sorted(surfaces)}")

    analytics = defs.get("CanonicalAnalyticsEvent") or {}
    event_schema = ((analytics.get("properties") or {}).get("event_name") or {})
    schema_events = set(event_schema.get("enum") or [])
    if schema_events != CANONICAL_EVENTS:
        errors.append(f"analytics event schema mismatch: {sorted(schema_events)}")

    for event in CANONICAL_EVENTS:
        if f'"{event}"' not in shared:
            errors.append(f"shared TypeScript contract missing analytics event {event}")

    required = set(analytics.get("required") or [])
    expected_required = {"event_name", "event_version", "occurred_at", "journey_id", "business_properties"}
    if required != expected_required:
        errors.append(f"analytics required properties mismatch: {sorted(required)}")

    if analytics.get("additionalProperties") is not False:
        errors.append("analytics event must reject unknown top-level properties")

    if "platform_extensions" not in shared or "platform_extensions" not in json.dumps(analytics):
        errors.append("platform diagnostics extension namespace is missing")
    else:
        extension_schema = (analytics.get("properties") or {}).get("platform_extensions") or {}
        if extension_schema.get("minProperties") != 1 or extension_schema.get("maxProperties") != 1:
            errors.append("platform_extensions must contain exactly one surface namespace when present")

    business_schema = (analytics.get("properties") or {}).get("business_properties") or {}
    forbidden_names = set((((business_schema.get("propertyNames") or {}).get("not") or {}).get("enum")) or [])
    for forbidden in {"raw_transcript", "transcript", "raw_audio", "raw_video"}:
        if forbidden not in forbidden_names:
            errors.append(f"analytics schema does not forbid sensitive payload key {forbidden}")

    if "sendWebAnalyticsEvent" not in web_analytics:
        errors.append("WEB analytics runtime sender is missing")
    if "sendNativeAnalyticsEvent" not in mobile_analytics:
        errors.append("native analytics runtime sender is missing")
    for surface in ("WEB", "IOS", "ANDROID"):
        if f'"{surface}"' not in analytics_e2e:
            errors.append(f"analytics parity E2E does not exercise {surface}")

    if 'source: "WEB"' not in web:
        errors.append("WEB deletion initiation does not bind source=WEB")
    if "NativeSurface" not in mobile or "source: platform" not in mobile:
        errors.append("native deletion initiation is not surface-bound")

    if errors:
        report(errors)

    print(
        "R1 MULTI-SURFACE GUARD: PASS "
        f"(surfaces={sorted(surfaces)}, analytics_events={sorted(schema_events)})"
    )


def report(errors: list[str]) -> None:
    print("R1 MULTI-SURFACE GUARD: FAIL", file=sys.stderr)
    for error in errors:
        print(f"ERROR: {error}", file=sys.stderr)
    raise SystemExit(1)


if __name__ == "__main__":
    main()
