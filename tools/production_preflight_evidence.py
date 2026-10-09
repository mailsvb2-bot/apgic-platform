#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
CANON_BINDING_FILES = (
    "canon/requirements/registry.yaml",
    "canon/requirements/coverage.json",
    "canon/requirements/approved-rfcs.yaml",
    "canon/baseline/APGIC_Requirement_Registry_v7_FINAL.yaml",
    "canon/baseline/APGIC_Canon_Coverage_v7_FINAL.json",
    "canon/APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx",
)

CONTRACT_BINDING_GLOBS = (
    "canon/contracts/*",
    "contracts/openapi/*",
    "contracts/jsonschema/*",
    "contracts/testing/*",
)

PROFILE_POLICY_FIELDS = {
    "R0": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
    ),
    "R1": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
    ),
    "R2": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
    ),
    "R3": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
        "booking_fulfillment_policy_path",
    ),
    "R4": (
        "jurisdiction_matrix_path",
        "retention_policy_path",
        "slo_policy_path",
        "provider_matrix_path",
        "market_cell_thresholds_path",
        "commerce_policy_path",
        "store_policy_path",
        "booking_fulfillment_policy_path",
        "provider_settlement_policy_path",
    ),
}


def fail(message: str) -> None:
    print(f"PRODUCTION PREFLIGHT EVIDENCE: FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def bind_artifacts(paths: list[Path]) -> dict[str, dict[str, str]]:
    bound: dict[str, dict[str, str]] = {}
    for path in sorted(paths, key=lambda item: item.relative_to(ROOT).as_posix()):
        if not path.is_file():
            fail(f"canonical binding artifact missing: {path.relative_to(ROOT)}")
        rel = path.relative_to(ROOT).as_posix()
        bound[rel] = {"sha256": sha256(path)}
    return bound


def manifest_sha256(bound: dict[str, dict[str, str]]) -> str:
    digest = hashlib.sha256()
    for rel, row in sorted(bound.items()):
        digest.update(rel.encode("utf-8"))
        digest.update(b"\0")
        digest.update(row["sha256"].encode("ascii"))
        digest.update(b"\n")
    return digest.hexdigest()


def build_canonical_binding() -> dict:
    requirement_paths = [ROOT / rel for rel in CANON_BINDING_FILES]
    requirement_artifacts = bind_artifacts(requirement_paths)

    registry_path = ROOT / "canon/requirements/registry.yaml"
    coverage_path = ROOT / "canon/requirements/coverage.json"
    registry = yaml.safe_load(registry_path.read_text(encoding="utf-8")) or {}
    coverage = json.loads(coverage_path.read_text(encoding="utf-8"))
    registry_meta = registry.get("meta") or {}
    coverage_meta = coverage.get("meta") or {}
    if registry_meta != coverage_meta:
        fail("working requirement registry and coverage meta differ")

    canon_rel = "canon/APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx"
    expected_canon_sha = registry_meta.get("canonical_docx_sha256")
    if not isinstance(expected_canon_sha, str) or requirement_artifacts[canon_rel]["sha256"] != expected_canon_sha:
        fail("canonical DOCX bytes do not match requirement registry meta")

    contract_paths: list[Path] = []
    for pattern in CONTRACT_BINDING_GLOBS:
        # Contract registries may contain nested schemas and versioned subdirectories.
        # A shallow glob silently omits them from the production revision hash.
        contract_paths.extend(path for path in ROOT.glob(pattern) if path.is_file())
        directory_pattern = pattern.removesuffix("/*")
        contract_paths.extend(
            path for path in (ROOT / directory_pattern).rglob("*") if path.is_file()
        )
    contract_paths = sorted(set(contract_paths))
    if not contract_paths:
        fail("contract/schema registry binding is empty")
    contract_artifacts = bind_artifacts(contract_paths)

    return {
        "canon_execution_baseline": registry_meta.get("canon_execution_baseline"),
        "registry_version": registry_meta.get("registry_version"),
        "canonical_docx_sha256": expected_canon_sha,
        "requirement_registry_revision": manifest_sha256(requirement_artifacts),
        "requirement_artifacts": requirement_artifacts,
        "contract_schema_registry_revision": manifest_sha256(contract_artifacts),
        "contract_schema_artifacts": contract_artifacts,
    }


def repo_file(raw: object, label: str) -> Path:
    if not isinstance(raw, str) or not raw.strip():
        fail(f"{label} is required")
    path = (ROOT / raw).resolve()
    if ROOT not in path.parents or not path.is_file():
        fail(f"{label} must resolve to an existing repository file")
    return path


def build_evidence(candidate_sha: str, profile: str, config_rel: str) -> dict:
    if not SHA_RE.fullmatch(candidate_sha):
        fail("candidate SHA must be a lowercase 40-character commit SHA")
    if profile not in PROFILE_POLICY_FIELDS:
        fail(f"unsupported profile {profile}")

    config_path = repo_file(config_rel, "launch config")
    config = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    if not isinstance(config, dict):
        fail("launch config must be a mapping")
    if config.get("environment") != "PRODUCTION":
        fail("launch config environment must be PRODUCTION")
    if config.get("production_approved") is not True:
        fail("launch config must be production_approved=true")

    policies: dict[str, dict[str, str]] = {}
    for field in PROFILE_POLICY_FIELDS[profile]:
        policy_path = repo_file(config.get(field), field)
        policies[field] = {
            "path": policy_path.relative_to(ROOT).as_posix(),
            "sha256": sha256(policy_path),
        }

    return {
        "schema_version": "production-preflight-evidence-v1",
        "evidence_class": "PRODUCTION_PREFLIGHT",
        "production_release": False,
        "production_promotion_eligible": True,
        "candidate_sha": candidate_sha,
        "release_profile": profile,
        "generated_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "launch_config": {
            "path": config_path.relative_to(ROOT).as_posix(),
            "sha256": sha256(config_path),
        },
        "policy_artifacts": policies,
        "canonical_binding": build_canonical_binding(),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--profile", choices=tuple(PROFILE_POLICY_FIELDS), required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    evidence = build_evidence(args.candidate_sha, args.profile, args.config)
    output = (ROOT / args.output).resolve()
    if ROOT not in output.parents:
        fail("output path escapes repository")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(
        json.dumps(evidence, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(
        "PRODUCTION PREFLIGHT EVIDENCE: PASS "
        f"(profile={args.profile}, candidate={args.candidate_sha}, output={output.relative_to(ROOT)})"
    )


if __name__ == "__main__":
    main()
