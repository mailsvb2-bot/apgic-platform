#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EVIDENCE_DIR = ROOT / "canon/evidence"
SCHEMA = EVIDENCE_DIR / "staging-runtime-evidence-v1.schema.json"
REGISTRY = ROOT / "canon/requirements/registry.yaml"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")

REQUIRED_LIMITATION_SNIPPETS = (
    "not production release approval",
    "APGIC-DR-001 remains unproven",
    "issue #3",
)


def _require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        errors.append(message)


def _registry_blocks(text: str) -> dict[str, str]:
    blocks: dict[str, str] = {}
    current_id: str | None = None
    current_lines: list[str] = []

    def flush() -> None:
        if current_id is not None:
            blocks[current_id] = "\n".join(current_lines)

    for line in text.splitlines():
        if line.startswith("- requirement_id: "):
            flush()
            current_id = line.split(": ", 1)[1].strip()
            current_lines = [line]
        elif current_id is not None:
            current_lines.append(line)
    flush()
    return blocks


def discover_evidence_paths() -> list[Path]:
    return sorted(
        path
        for path in EVIDENCE_DIR.glob("staging-runtime-*.json")
        if path.name != SCHEMA.name
    )


def _validate_artifact(
    evidence_path: Path,
    schema: dict,
    blocks: dict[str, str],
) -> list[str]:
    errors: list[str] = []
    ref = f"canon/evidence/{evidence_path.name}"

    try:
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return [f"{ref}: unreadable JSON: {exc}"]

    required = set(schema.get("required", []))
    properties = set(schema.get("properties", {}))
    actual = set(evidence)
    _require(not (required - actual), f"{ref}: missing required fields: {sorted(required - actual)}", errors)
    if schema.get("additionalProperties") is False:
        _require(not (actual - properties), f"{ref}: undeclared fields: {sorted(actual - properties)}", errors)

    _require(evidence.get("schema_version") == "staging-runtime-evidence-v1",
             f"{ref}: unexpected staging evidence schema version", errors)
    _require(evidence.get("evidence_class") == "STAGING_RUNTIME",
             f"{ref}: unexpected staging evidence class", errors)
    _require(evidence.get("production_release") is False,
             f"{ref}: staging evidence must never claim production release", errors)
    _require(evidence.get("environment") == "staging",
             f"{ref}: staging evidence environment mismatch", errors)
    _require(evidence.get("public_host") == "apgic.ru",
             f"{ref}: public host mismatch", errors)

    candidate_sha = evidence.get("candidate_sha", "")
    _require(isinstance(candidate_sha, str) and SHA_RE.fullmatch(candidate_sha) is not None,
             f"{ref}: candidate SHA is not a full lowercase commit SHA", errors)
    deployment_identity = evidence.get("deployment_identity", "")
    _require(
        isinstance(deployment_identity, str)
        and len(candidate_sha) >= 8
        and candidate_sha[:8] in deployment_identity,
        f"{ref}: deployment identity is not bound to candidate SHA",
        errors,
    )

    dns = evidence.get("dns", {})
    nameservers = dns.get("authoritative_nameservers", [])
    resolvers = dns.get("recursive_resolvers_checked", [])
    _require(isinstance(nameservers, list) and len(nameservers) >= 4 and len(nameservers) == len(set(nameservers)),
             f"{ref}: authoritative nameserver evidence is incomplete or duplicated", errors)
    _require(dns.get("apex_ipv4") == "92.51.23.254", f"{ref}: apex IPv4 mismatch", errors)
    _require(dns.get("www_ipv4") == "92.51.23.254", f"{ref}: www IPv4 mismatch", errors)
    _require(isinstance(resolvers, list) and len(resolvers) >= 2 and len(resolvers) == len(set(resolvers)),
             f"{ref}: recursive resolver evidence is incomplete or duplicated", errors)

    tls = evidence.get("tls", {})
    _require(bool(tls.get("issuer")), f"{ref}: TLS issuer is missing", errors)
    _require(set(tls.get("sans", [])) == {"apgic.ru", "www.apgic.ru"},
             f"{ref}: TLS SAN set mismatch", errors)
    _require(tls.get("http_redirect") == 301, f"{ref}: HTTP redirect evidence mismatch", errors)
    _require(tls.get("https_apex_status") == 200, f"{ref}: HTTPS apex status mismatch", errors)
    _require(tls.get("https_www_status") == 200, f"{ref}: HTTPS www status mismatch", errors)

    runtime = evidence.get("runtime", {})
    _require(runtime.get("meta_commit_sha") == candidate_sha,
             f"{ref}: runtime meta SHA mismatch", errors)
    _require(runtime.get("healthz") == "ok", f"{ref}: healthz evidence is not ok", errors)
    _require(runtime.get("readyz") == "ready", f"{ref}: readyz evidence is not ready", errors)
    _require(runtime.get("watchdog_result") == "success",
             f"{ref}: runtime watchdog evidence is not success", errors)
    _require(runtime.get("nginx_config_valid") is True,
             f"{ref}: nginx config evidence is not valid", errors)
    _require(runtime.get("worktree_clean") is True,
             f"{ref}: deployed worktree evidence is not clean", errors)

    database = evidence.get("database", {})
    _require(database.get("backup_result") == "success",
             f"{ref}: staging backup evidence is not success", errors)
    _require(bool(database.get("backup_artifact")),
             f"{ref}: staging backup artifact is missing", errors)
    _require(database.get("restore_verification_result") == "success",
             f"{ref}: staging restore verification evidence is not success", errors)
    _require(
        isinstance(database.get("restored_table_count"), int)
        and database.get("restored_table_count", 0) > 0,
        f"{ref}: restored table count is invalid",
        errors,
    )

    live_e2e = evidence.get("live_e2e", {})
    _require(live_e2e.get("spec_path") == "apps/web/e2e/foundation.spec.ts",
             f"{ref}: live E2E spec mismatch", errors)
    _require(live_e2e.get("base_url") == "https://apgic.ru",
             f"{ref}: live E2E base URL mismatch", errors)
    _require(set(live_e2e.get("projects", [])) == {"phone", "tablet", "desktop"},
             f"{ref}: live E2E project set mismatch", errors)
    _require(live_e2e.get("tests_total") == 12,
             f"{ref}: live E2E total must remain 12", errors)
    _require(live_e2e.get("tests_passed") == 12,
             f"{ref}: live E2E pass count mismatch", errors)
    _require(live_e2e.get("tests_failed") == 0,
             f"{ref}: live E2E failure count must be zero", errors)
    _require(live_e2e.get("tested_sha") == candidate_sha,
             f"{ref}: live E2E tested SHA mismatch", errors)

    supported_list = evidence.get("supported_requirements", [])
    supported = set(supported_list) if isinstance(supported_list, list) else set()
    _require(bool(supported), f"{ref}: supported requirement set is empty", errors)
    _require(len(supported) == len(supported_list),
             f"{ref}: supported requirement set contains duplicates", errors)
    _require("APGIC-DR-001" not in supported,
             f"{ref}: staging evidence must not claim APGIC-DR-001 production restore proof", errors)

    limitations = "\n".join(evidence.get("limitations", []))
    for snippet in REQUIRED_LIMITATION_SNIPPETS:
        _require(snippet in limitations, f"{ref}: required limitation missing: {snippet}", errors)

    for requirement_id in supported:
        block = blocks.get(requirement_id)
        _require(block is not None, f"{ref}: registry requirement missing: {requirement_id}", errors)
        if block is not None:
            _require(ref in block,
                     f"{ref}: registry requirement missing evidence ref: {requirement_id}", errors)

    for requirement_id, block in blocks.items():
        if ref in block and requirement_id not in supported:
            errors.append(f"{ref}: evidence over-claimed by unsupported requirement: {requirement_id}")

    return errors


def validate() -> list[str]:
    errors: list[str] = []
    _require(SCHEMA.is_file(), "staging runtime evidence schema is missing", errors)
    _require(REGISTRY.is_file(), "requirement registry is missing", errors)
    if errors:
        return errors

    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    registry = REGISTRY.read_text(encoding="utf-8")
    blocks = _registry_blocks(registry)

    _require(
        schema.get("properties", {}).get("production_release", {}).get("const") is False,
        "schema must fail closed with production_release=false",
        errors,
    )

    evidence_paths = discover_evidence_paths()
    _require(bool(evidence_paths), "no staging runtime evidence artifacts discovered", errors)
    for evidence_path in evidence_paths:
        errors.extend(_validate_artifact(evidence_path, schema, blocks))
    return errors


def main() -> int:
    errors = validate()
    if errors:
        print("STAGING RUNTIME EVIDENCE GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print(f"STAGING RUNTIME EVIDENCE GUARD: PASS ({len(discover_evidence_paths())} artifacts)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
