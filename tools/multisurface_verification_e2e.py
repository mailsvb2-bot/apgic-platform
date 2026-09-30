#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import shutil
import subprocess
import sys
import tempfile

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
REQUIREMENT_ID = "APGIC-MOBILE-003"
SURFACE_PROOFS = [
    "surface://WEB/mobile003-e2e-proof",
    "surface://IOS/mobile003-e2e-proof",
    "surface://ANDROID/mobile003-e2e-proof",
]


def run_guard(sandbox: pathlib.Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, "tools/multisurface_guard.py"],
        cwd=sandbox,
        text=True,
        capture_output=True,
        timeout=60,
        check=False,
    )


def write_registry(path: pathlib.Path, registry: dict) -> None:
    path.write_text(
        yaml.safe_dump(registry, allow_unicode=True, sort_keys=False),
        encoding="utf-8",
    )


def main() -> None:
    with tempfile.TemporaryDirectory(prefix="apgic-multisurface-verification-") as tmp:
        sandbox = pathlib.Path(tmp) / "repo"
        shutil.copytree(
            ROOT,
            sandbox,
            ignore=shutil.ignore_patterns(
                ".git",
                "node_modules",
                ".next",
                "__pycache__",
                "*.pyc",
            ),
        )

        registry_path = sandbox / "canon/requirements/registry.yaml"
        registry = yaml.safe_load(registry_path.read_text(encoding="utf-8"))
        requirements = registry.get("requirements") or []
        target = next(
            (item for item in requirements if item.get("requirement_id") == REQUIREMENT_ID),
            None,
        )
        if target is None:
            raise SystemExit(f"{REQUIREMENT_ID} is missing from Requirement Registry")

        target["status"] = "VERIFIED"
        target["evidence_refs"] = [
            ref
            for ref in (target.get("evidence_refs") or [])
            if not str(ref).startswith("surface://")
        ]
        write_registry(registry_path, registry)

        denied = run_guard(sandbox)
        denied_output = denied.stdout + denied.stderr
        if denied.returncode == 0:
            raise SystemExit(
                "MULTI-SURFACE VERIFICATION E2E: FAIL: "
                "WEB_ONLY-style VERIFIED requirement unexpectedly passed"
            )
        if REQUIREMENT_ID not in denied_output or "surface evidence/approved exception" not in denied_output:
            raise SystemExit(
                "MULTI-SURFACE VERIFICATION E2E: FAIL: "
                f"unexpected denial diagnostic:\n{denied_output}"
            )

        target["evidence_refs"].extend(SURFACE_PROOFS)
        write_registry(registry_path, registry)

        allowed = run_guard(sandbox)
        allowed_output = allowed.stdout + allowed.stderr
        if allowed.returncode != 0:
            raise SystemExit(
                "MULTI-SURFACE VERIFICATION E2E: FAIL: "
                f"complete WEB+iOS+Android proof did not pass:\n{allowed_output}"
            )

    print(
        "MULTI-SURFACE VERIFICATION E2E: PASS "
        "(VERIFIED requires WEB+IOS+ANDROID evidence or governed exception)"
    )


if __name__ == "__main__":
    main()
