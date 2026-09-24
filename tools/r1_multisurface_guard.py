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
    for path in (SHARED, SCHEMA, WEB, MOBILE):
        if not path.is_file():
            errors.append(f"missing R1 multi-surface contract file: {path.relative_to(ROOT)}")

    if errors:
        report(errors)

    shared = SHARED.read_text(encoding="utf-8")
    web = WEB.read_text(encoding="utf-8")
    mobile = MOBILE.read_text(encoding="utf-8")
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

    if "platform_extensions" not in shared or "platform_extensions" not in json.dumps(analytics):
        errors.append("platform diagnostics extension namespace is missing")

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
