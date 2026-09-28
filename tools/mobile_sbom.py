#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import sys
import uuid
from pathlib import Path
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[1]
PACKAGE = ROOT / "apps/mobile/package.json"


class SBOMError(ValueError):
    pass


def component_ref(name: str, version: str) -> str:
    return f"pkg:npm/{quote(name, safe='')}@{quote(version, safe='')}"


def _resolved_version(name: str, node: dict) -> str | None:
    if not isinstance(name, str) or not name:
        raise SBOMError("installed dependency has no name")
    version = node.get("version")
    if isinstance(version, str) and version:
        return version

    installed_markers = ("path", "resolved", "integrity")
    if node.get("missing") is True or not any(node.get(key) for key in installed_markers):
        return None
    raise SBOMError(f"{name}: installed dependency has no resolved version")


def build_cyclonedx_sbom(package_doc: dict, tree_doc: dict) -> dict:
    problems = tree_doc.get("problems") or []
    if problems:
        raise SBOMError(f"npm dependency graph contains problems: {problems}")

    package_name = package_doc.get("name")
    package_version = package_doc.get("version")
    if not isinstance(package_name, str) or not package_name:
        raise SBOMError("mobile package has no name")
    if not isinstance(package_version, str) or not package_version:
        raise SBOMError("mobile package has no version")

    declared: dict[str, str] = {}
    for section in ("dependencies", "devDependencies"):
        values = package_doc.get(section) or {}
        if not isinstance(values, dict):
            raise SBOMError(f"{section} must be an object")
        declared.update({str(name): str(version) for name, version in values.items()})

    root_dependencies = tree_doc.get("dependencies") or {}
    missing = sorted(set(declared) - set(root_dependencies))
    if missing:
        raise SBOMError(f"installed graph is missing declared dependencies: {missing}")

    components: dict[str, dict] = {}
    edges: dict[str, set[str]] = {}

    def walk(dependencies: dict) -> set[str]:
        refs: set[str] = set()
        for name in sorted(dependencies):
            node = dependencies[name] or {}
            if not isinstance(node, dict):
                raise SBOMError(f"{name}: npm dependency node must be an object")
            version = _resolved_version(name, node)
            if version is None:
                continue
            dep_name = name
            ref = component_ref(dep_name, version)
            refs.add(ref)
            component = {
                "type": "library",
                "name": dep_name,
                "version": version,
                "bom-ref": ref,
                "purl": ref,
            }
            resolved = node.get("resolved")
            if isinstance(resolved, str) and resolved:
                component["externalReferences"] = [
                    {"type": "distribution", "url": resolved}
                ]
            components.setdefault(ref, component)

            children = node.get("dependencies") or {}
            if not isinstance(children, dict):
                raise SBOMError(f"{dep_name}@{version}: dependencies must be an object")
            child_refs = walk(children)
            edges.setdefault(ref, set()).update(child_refs)
        return refs

    root_refs = walk(root_dependencies)
    root_ref = f"application:{package_name}@{package_version}"
    edges[root_ref] = set(root_refs)

    canonical_tree = json.dumps(tree_doc, sort_keys=True, separators=(",", ":"))
    digest = hashlib.sha256(canonical_tree.encode("utf-8")).hexdigest()
    serial = uuid.uuid5(uuid.NAMESPACE_URL, f"apgic-mobile-npm-tree:{digest}")

    dependency_rows = [
        {"ref": ref, "dependsOn": sorted(edges.get(ref, set()))}
        for ref in sorted({root_ref, *components})
    ]

    return {
        "bomFormat": "CycloneDX",
        "specVersion": "1.6",
        "serialNumber": f"urn:uuid:{serial}",
        "version": 1,
        "metadata": {
            "component": {
                "type": "application",
                "name": package_name,
                "version": package_version,
                "bom-ref": root_ref,
            },
            "properties": [
                {"name": "apgic:source", "value": "npm ls --json --all"},
                {"name": "apgic:graph_sha256", "value": digest},
            ],
        },
        "components": [components[ref] for ref in sorted(components)],
        "dependencies": dependency_rows,
    }


def validate_sbom(sbom: dict, package_doc: dict) -> list[str]:
    errors: list[str] = []
    if sbom.get("bomFormat") != "CycloneDX":
        errors.append("SBOM format must be CycloneDX")
    if sbom.get("specVersion") != "1.6":
        errors.append("SBOM specVersion must be 1.6")

    metadata = sbom.get("metadata") or {}
    root = metadata.get("component") or {}
    if root.get("name") != package_doc.get("name"):
        errors.append("SBOM root component name differs from mobile package")
    if root.get("version") != package_doc.get("version"):
        errors.append("SBOM root component version differs from mobile package")

    components = sbom.get("components")
    if not isinstance(components, list) or not components:
        errors.append("SBOM must contain installed components")
        return errors

    refs: set[str] = set()
    for component in components:
        ref = component.get("bom-ref")
        if not isinstance(ref, str) or not ref:
            errors.append("SBOM component missing bom-ref")
            continue
        if ref in refs:
            errors.append(f"duplicate SBOM component ref: {ref}")
        refs.add(ref)
        if not component.get("name") or not component.get("version"):
            errors.append(f"{ref}: component missing name/version")

    dependency_rows = sbom.get("dependencies")
    if not isinstance(dependency_rows, list):
        errors.append("SBOM dependency graph missing")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tree", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--package", type=Path, default=PACKAGE)
    args = parser.parse_args()

    try:
        package_doc = json.loads(args.package.read_text(encoding="utf-8"))
        tree_doc = json.loads(args.tree.read_text(encoding="utf-8"))
        sbom = build_cyclonedx_sbom(package_doc, tree_doc)
        errors = validate_sbom(sbom, package_doc)
    except (OSError, json.JSONDecodeError, SBOMError) as exc:
        print(f"MOBILE SBOM: FAIL: {exc}", file=sys.stderr)
        return 1

    if errors:
        print("MOBILE SBOM: FAIL", file=sys.stderr)
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(sbom, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"MOBILE SBOM: PASS ({len(sbom['components'])} installed components)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
