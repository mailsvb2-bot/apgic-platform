#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
REGISTRY = ROOT / "canon/requirements/registry.yaml"
CI_EVIDENCE_MAP = ROOT / "canon/evidence/r0-ci-evidence-map.json"

FIXED_ARTIFACTS = [
    "canon/requirements/registry.yaml",
    "canon/requirements/coverage.json",
    "canon/evidence/r0-ci-evidence-map.json",
    "canon/evidence/release-evidence-v2.schema.json",
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

REQUIREMENT_BLOCK = re.compile(
    r"(?ms)^- requirement_id: (APGIC-[A-Z0-9-]+)\n(.*?)(?=^- requirement_id: |\Z)"
)
SURFACE_EVIDENCE_REF = re.compile(
    r"^surface://(?:WEB|PWA|IOS|ANDROID)/[A-Za-z0-9._/-]+$"
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def fail(message: str) -> None:
    print(f"RELEASE EVIDENCE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def _list_from_block(block: str, key: str, *, indent: int = 2) -> list[str]:
    prefix = " " * indent
    item_prefix = " " * indent + "- "
    pattern = re.compile(
        rf"(?m)^{re.escape(prefix + key + ':')}\n((?:{re.escape(item_prefix)}.+\n?)*)"
    )
    match = pattern.search(block)
    if not match:
        return []
    return [
        line[len(item_prefix):].strip()
        for line in match.group(1).splitlines()
        if line.startswith(item_prefix)
    ]


def load_registry_requirements(path: Path = REGISTRY) -> dict[str, dict[str, Any]]:
    text = path.read_text(encoding="utf-8")
    requirements: dict[str, dict[str, Any]] = {}
    for match in REQUIREMENT_BLOCK.finditer(text):
        requirement_id = match.group(1)
        block = match.group(0)
        release_match = re.search(r"(?m)^  release_profile: (R\d+)$", block)
        required_match = re.search(
            r"(?m)^    required_evidence:\n((?:    - .+\n?)*)",
            block,
        )
        required_evidence = []
        if required_match:
            required_evidence = [
                line[6:].strip()
                for line in required_match.group(1).splitlines()
                if line.startswith("    - ")
            ]
        requirements[requirement_id] = {
            "release_profile": release_match.group(1) if release_match else None,
            "implementation_refs": _list_from_block(block, "implementation_refs"),
            "contract_refs": _list_from_block(block, "contract_refs"),
            "test_refs": _list_from_block(block, "test_refs"),
            "evidence_refs": _list_from_block(block, "evidence_refs"),
            "required_evidence": required_evidence,
        }
    return requirements


def load_ci_evidence_map(path: Path = CI_EVIDENCE_MAP) -> dict[str, Any]:
    document = json.loads(path.read_text(encoding="utf-8"))
    if document.get("schema_version") != "r0-ci-evidence-map-v1":
        fail("unsupported CI evidence map schema_version")
    if document.get("release_profile") != "R0":
        fail("CI evidence map must target R0")
    if not isinstance(document.get("allowed_gates"), list) or not document["allowed_gates"]:
        fail("CI evidence map allowed_gates must be a non-empty list")
    if not isinstance(document.get("requirements"), dict) or not document["requirements"]:
        fail("CI evidence map requirements must be a non-empty object")
    return document


def build_requirement_evidence(
    gate_results: dict[str, str],
    *,
    registry: dict[str, dict[str, Any]] | None = None,
    evidence_map: dict[str, Any] | None = None,
) -> dict[str, dict[str, Any]]:
    registry = registry or load_registry_requirements()
    evidence_map = evidence_map or load_ci_evidence_map()

    allowed_gates = set(evidence_map["allowed_gates"])
    supplied_gates = set(gate_results)
    unknown_supplied = supplied_gates - allowed_gates
    if unknown_supplied:
        fail(f"gate results contain gates not declared by evidence map: {sorted(unknown_supplied)}")

    r0_requirements = {
        requirement_id
        for requirement_id, requirement in registry.items()
        if requirement.get("release_profile") == "R0"
    }
    mapped_requirements = set(evidence_map["requirements"])
    if mapped_requirements != r0_requirements:
        missing = sorted(r0_requirements - mapped_requirements)
        extra = sorted(mapped_requirements - r0_requirements)
        fail(
            "CI evidence map must cover exactly all R0 requirements "
            f"missing={missing} extra={extra}"
        )

    output: dict[str, dict[str, Any]] = {}
    for requirement_id, mapping in sorted(evidence_map["requirements"].items()):
        requirement = registry.get(requirement_id)
        if requirement is None:
            fail(f"CI evidence map references unknown requirement {requirement_id}")
        if requirement.get("release_profile") != "R0":
            fail(f"CI evidence map references non-R0 requirement {requirement_id}")

        claims = mapping.get("claims")
        unproven = mapping.get("unproven_evidence")
        if not isinstance(claims, dict):
            fail(f"{requirement_id}: claims must be an object")
        if not isinstance(unproven, list) or any(not isinstance(item, str) for item in unproven):
            fail(f"{requirement_id}: unproven_evidence must be a string list")

        required = set(requirement.get("required_evidence") or [])
        claimed = set(claims)
        unproven_set = set(unproven)
        if claimed & unproven_set:
            fail(f"{requirement_id}: evidence kind is both claimed and unproven")
        if claimed | unproven_set != required:
            missing = sorted(required - claimed - unproven_set)
            extra = sorted((claimed | unproven_set) - required)
            fail(
                f"{requirement_id}: evidence partition mismatch "
                f"missing={missing} extra={extra}"
            )

        traceability_refs = set(requirement.get("implementation_refs") or [])
        traceability_refs.update(requirement.get("contract_refs") or [])
        traceability_refs.update(requirement.get("test_refs") or [])
        traceability_refs.update(requirement.get("evidence_refs") or [])

        proven: dict[str, dict[str, list[str]]] = {}
        for evidence_kind, claim in sorted(claims.items()):
            if not isinstance(claim, dict):
                fail(f"{requirement_id}/{evidence_kind}: claim must be an object")
            gates = claim.get("gates")
            proof_refs = claim.get("proof_refs")
            if not isinstance(gates, list) or not gates:
                fail(f"{requirement_id}/{evidence_kind}: gates must be non-empty")
            if not isinstance(proof_refs, list) or not proof_refs:
                fail(f"{requirement_id}/{evidence_kind}: proof_refs must be non-empty")
            if len(gates) != len(set(gates)):
                fail(f"{requirement_id}/{evidence_kind}: duplicate gates")
            if len(proof_refs) != len(set(proof_refs)):
                fail(f"{requirement_id}/{evidence_kind}: duplicate proof refs")

            for gate in gates:
                if gate not in allowed_gates:
                    fail(f"{requirement_id}/{evidence_kind}: unknown gate {gate}")
                result = gate_results.get(gate)
                if result is None:
                    fail(f"{requirement_id}/{evidence_kind}: gate result missing for {gate}")
                if result != "success":
                    fail(
                        f"{requirement_id}/{evidence_kind}: gate {gate} is {result}, "
                        "cannot claim evidence"
                    )

            for ref in proof_refs:
                if ref not in traceability_refs:
                    fail(
                        f"{requirement_id}/{evidence_kind}: proof ref is outside "
                        f"requirement traceability: {ref}"
                    )
                if ref.startswith("surface://"):
                    if not SURFACE_EVIDENCE_REF.fullmatch(ref):
                        fail(
                            f"{requirement_id}/{evidence_kind}: malformed surface evidence ref: {ref}"
                        )
                    continue
                candidate = ROOT / ref.rstrip("/")
                if not candidate.exists():
                    fail(f"{requirement_id}/{evidence_kind}: proof ref does not exist: {ref}")

            proven[evidence_kind] = {
                "gates": list(gates),
                "proof_refs": list(proof_refs),
            }

        if proven and not unproven:
            status = "CI_PROVEN"
        elif proven:
            status = "PARTIAL"
        else:
            status = "UNPROVEN"

        output[requirement_id] = {
            "status": status,
            "proven_evidence": proven,
            "unproven_evidence": sorted(unproven),
        }

    return output


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

    requirement_evidence = build_requirement_evidence(gate_results)

    artifacts: dict[str, dict[str, str]] = {}
    paths = [ROOT / item for item in FIXED_ARTIFACTS]
    paths.extend(sorted((ROOT / "backend/migrations").glob("*.sql")))

    for path in paths:
        if not path.is_file():
            fail(f"required artifact missing: {path.relative_to(ROOT)}")
        rel = path.relative_to(ROOT).as_posix()
        artifacts[rel] = {"sha256": sha256(path)}

    document = {
        "schema_version": "release-evidence-v2",
        "evidence_class": "CI_CANDIDATE",
        "production_release": False,
        "candidate_sha": args.candidate_sha,
        "generated_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "surfaces": ["WEB", "PWA", "IOS", "ANDROID"],
        "gate_results": gate_results,
        "requirement_evidence": requirement_evidence,
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
    print(
        f"RELEASE EVIDENCE: PASS ({output.relative_to(ROOT)}, "
        f"{len(requirement_evidence)} requirement mappings)"
    )


if __name__ == "__main__":
    main()
