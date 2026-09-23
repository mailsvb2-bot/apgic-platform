#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any

import yaml

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "canon/contracts/client-surface-manifest.yaml"
REGISTRY = ROOT / "canon/requirements/registry.yaml"

DECLARATION_PATTERNS = (
    re.compile(r"\btype\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
    re.compile(r"\binterface\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
    re.compile(r"\bclass\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
    re.compile(r"\bconst\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
)


def load_yaml(path: Path) -> dict[str, Any]:
    document = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(document, dict):
        raise ValueError(f"{path.relative_to(ROOT)} must be a mapping")
    return document


def requirement_refs(requirement: dict[str, Any]) -> list[str]:
    refs: list[str] = []
    for key in ("implementation_refs", "contract_refs", "test_refs", "evidence_refs"):
        values = requirement.get(key) or []
        if isinstance(values, list):
            refs.extend(str(value) for value in values)
    return refs


def approved_exception_surfaces(
    exceptions: list[dict[str, Any]],
    requirement_id: str,
) -> set[str]:
    covered: set[str] = set()
    for item in exceptions:
        if item.get("requirement_id") != requirement_id:
            continue
        if item.get("status") != "APPROVED":
            continue
        if not item.get("exception_id") or not item.get("reason") or not item.get("approved_by"):
            continue
        surfaces = item.get("surfaces") or []
        if isinstance(surfaces, list):
            covered.update(str(surface) for surface in surfaces)
    return covered


def launch_surface_errors(
    requirements: list[dict[str, Any]],
    exceptions: list[dict[str, Any]],
    launch_profiles: set[str],
    evidence_prefixes: dict[str, str],
) -> list[str]:
    errors: list[str] = []
    required = set(evidence_prefixes)

    for requirement in requirements:
        if requirement.get("release_profile") not in launch_profiles:
            continue
        if requirement.get("status") not in {"VERIFIED", "RELEASED"}:
            continue

        requirement_id = str(requirement.get("requirement_id"))
        refs = requirement_refs(requirement)
        present = {
            surface
            for surface, prefix in evidence_prefixes.items()
            if any(ref.startswith(prefix) for ref in refs)
        }
        covered = approved_exception_surfaces(exceptions, requirement_id)
        missing = sorted(required - present - covered)
        if missing:
            errors.append(
                f"{requirement_id}: {requirement.get('status')} without "
                f"surface evidence/approved exception for {missing}"
            )
    return errors


def main() -> None:
    errors: list[str] = []

    try:
        manifest = load_yaml(MANIFEST)
        registry = load_yaml(REGISTRY)
        exception_path = ROOT / str(manifest.get("compatibility_exceptions", ""))
        exceptions_doc = load_yaml(exception_path)
    except (OSError, yaml.YAMLError, ValueError) as exc:
        print(f"MULTI-SURFACE GUARD: FAIL: {exc}", file=sys.stderr)
        raise SystemExit(1)

    if manifest.get("server_truth_owner") != "backend":
        errors.append("server_truth_owner must be backend")

    required_surfaces = manifest.get("required_surfaces") or []
    if set(required_surfaces) != {"WEB", "PWA", "IOS", "ANDROID"}:
        errors.append("required_surfaces must be exactly WEB/PWA/IOS/ANDROID")

    contract = manifest.get("contract") or {}
    source_path = ROOT / str(contract.get("source", ""))
    generated_path = ROOT / str(contract.get("generated", ""))
    if not source_path.is_file():
        errors.append("OpenAPI source contract is missing")
    if not generated_path.is_file():
        errors.append("generated TypeScript contract is missing")

    if source_path.is_file():
        openapi = load_yaml(source_path)
        surface_schema = (
            ((openapi.get("components") or {}).get("schemas") or {}).get("Surface") or {}
        )
        if set(surface_schema.get("enum") or []) != set(required_surfaces):
            errors.append("OpenAPI Surface enum differs from required_surfaces")

    consumers = manifest.get("consumers") or {}
    if set(consumers) != set(required_surfaces):
        errors.append("consumer manifest must declare all required surfaces")
    generated_fragment = "packages/contracts/src/generated/apgic-v1"
    for surface in required_surfaces:
        raw_path = consumers.get(surface)
        path = ROOT / str(raw_path or "")
        if not path.is_file():
            errors.append(f"{surface}: client contract consumer file missing")
            continue
        content = path.read_text(encoding="utf-8", errors="ignore")
        if generated_fragment not in content:
            errors.append(f"{surface}: consumer does not bind to generated APGIC contract")

    shared_roots = {str(value) for value in (manifest.get("shared_code_roots") or [])}
    packages_root = ROOT / "packages"
    if packages_root.is_dir():
        for child in packages_root.iterdir():
            if child.is_dir():
                rel = child.relative_to(ROOT).as_posix()
                if rel not in shared_roots:
                    errors.append(f"unregistered shared-code root: {rel}")

    mobile_package = json.loads((ROOT / "apps/mobile/package.json").read_text(encoding="utf-8"))
    native = manifest.get("native") or {}
    runtime_dependency = native.get("runtime_dependency")
    language_dependency = native.get("language_dependency")
    if runtime_dependency not in (mobile_package.get("dependencies") or {}):
        errors.append(f"native runtime dependency missing: {runtime_dependency}")
    if language_dependency not in (mobile_package.get("devDependencies") or {}):
        errors.append(f"native language dependency missing: {language_dependency}")

    web_package = json.loads((ROOT / "apps/web/package.json").read_text(encoding="utf-8"))
    if "next" not in (web_package.get("dependencies") or {}):
        errors.append("web canonical Next.js dependency missing")
    if "typescript" not in (web_package.get("devDependencies") or {}):
        errors.append("web canonical TypeScript dependency missing")

    forbidden_identifiers = set(manifest.get("forbidden_client_truth_identifiers") or [])
    for app_root in (ROOT / "apps/web", ROOT / "apps/mobile"):
        for path in app_root.rglob("*"):
            if not path.is_file() or path.suffix.lower() not in {".ts", ".tsx", ".js", ".jsx"}:
                continue
            content = path.read_text(encoding="utf-8", errors="ignore")
            declarations: set[str] = set()
            for pattern in DECLARATION_PATTERNS:
                declarations.update(pattern.findall(content))
            forbidden = sorted(declarations & forbidden_identifiers)
            if forbidden:
                errors.append(
                    f"{path.relative_to(ROOT).as_posix()}: client-owned server truth "
                    f"declarations forbidden: {forbidden}"
                )

    launch_profiles = set(manifest.get("launch_profiles") or [])
    prefix_doc = manifest.get("verification_surface_evidence_prefixes") or {}
    evidence_prefixes = {
        str(surface): str(prefix)
        for surface, prefix in prefix_doc.items()
        if str(surface) in {"WEB", "IOS", "ANDROID"}
    }
    if set(evidence_prefixes) != {"WEB", "IOS", "ANDROID"}:
        errors.append("verification surface evidence prefixes must cover WEB/IOS/ANDROID")

    requirements = registry.get("requirements") or []
    exceptions = exceptions_doc.get("exceptions") or []
    if not isinstance(requirements, list):
        errors.append("requirements registry is malformed")
        requirements = []
    if not isinstance(exceptions, list):
        errors.append("compatibility exception registry is malformed")
        exceptions = []

    errors.extend(
        launch_surface_errors(
            requirements,
            exceptions,
            launch_profiles,
            evidence_prefixes,
        )
    )

    if errors:
        print("MULTI-SURFACE GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        raise SystemExit(1)

    print(
        "MULTI-SURFACE GUARD: PASS "
        f"(surfaces={len(required_surfaces)}, launch_requirements={len(requirements)})"
    )


if __name__ == "__main__":
    main()
