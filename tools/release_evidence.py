#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

FIXED_ARTIFACTS = [
    "canon/requirements/registry.yaml",
    "canon/requirements/coverage.json",
    "contracts/openapi/apgic-v1.yaml",
    "contracts/jsonschema/mobile-policy-v1.schema.json",
    "config/launch.ci.yaml",
    "config/jurisdiction/r0-ci-matrix.yaml",
    "config/retention/r0-ci-matrix.yaml",
    "config/slo/r0-ci-policy.yaml",
    "config/providers/r0-ci-matrix.yaml",
    "apps/web/package.json",
    "apps/mobile/package.json",
    "apps/mobile/dependency-registry.yaml",
    "apps/mobile/privacy-declaration.yaml",
    "apps/mobile/android/app/build.gradle",
    "apps/mobile/android/gradle/wrapper/gradle-wrapper.properties",
    "apps/mobile/android/data-safety.yaml",
    "apps/mobile/ios/Podfile",
    "apps/mobile/ios/APGIC.xcodeproj/project.pbxproj",
    "apps/mobile/ios/APGIC/PrivacyInfo.xcprivacy",
]

def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()

def fail(message: str) -> None:
    print(f"RELEASE EVIDENCE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--gate", action="append", default=[])
    args = parser.parse_args()

    gate_results: dict[str, str] = {}
    for raw in args.gate:
        if "=" not in raw:
            fail(f"invalid --gate value {raw!r}")
        name, result = raw.split("=", 1)
        if not name or result not in {"success", "failure", "cancelled", "skipped"}:
            fail(f"invalid gate result {raw!r}")
        gate_results[name] = result

    if not gate_results:
        fail("at least one gate result is required")
    failed = {name: result for name, result in gate_results.items() if result != "success"}
    if failed:
        fail(f"release evidence cannot be created from non-green gates: {failed}")

    artifacts: dict[str, dict[str, str]] = {}
    paths = [ROOT / item for item in FIXED_ARTIFACTS]
    paths.extend(sorted((ROOT / "backend/migrations").glob("*.sql")))

    for path in paths:
        if not path.is_file():
            fail(f"required artifact missing: {path.relative_to(ROOT)}")
        rel = path.relative_to(ROOT).as_posix()
        artifacts[rel] = {"sha256": sha256(path)}

    document = {
        "schema_version": "release-evidence-v1",
        "evidence_class": "CI_CANDIDATE",
        "production_release": False,
        "candidate_sha": args.candidate_sha,
        "generated_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "surfaces": ["WEB", "PWA", "IOS", "ANDROID"],
        "gate_results": gate_results,
        "artifacts": artifacts,
    }

    output = (ROOT / args.output).resolve()
    if ROOT not in output.parents:
        fail("output path escapes repository")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(
        json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(f"RELEASE EVIDENCE: PASS ({output.relative_to(ROOT)})")

if __name__ == "__main__":
    main()
