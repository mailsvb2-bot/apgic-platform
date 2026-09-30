#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO_CREDENTIAL = ROOT / "backend/internal/security/service_credential.go"
SCHEMA = ROOT / "contracts/jsonschema/service-credential-metadata-v1.schema.json"
EXECUTOR = ROOT / "backend/internal/connector/executor.go"


def validate(go_text: str, schema: dict, executor_text: str) -> list[str]:
    errors: list[str] = []
    required = {
        "principal_id",
        "credential_version",
        "not_before",
        "expires_at",
    }
    properties = set(schema.get("properties", {}))
    if set(schema.get("required", [])) != required:
        errors.append("credential metadata required fields drifted")
    forbidden = {"secret", "secret_digest", "secret_hash", "token", "credential"}
    leaked = sorted(name for name in properties if name in forbidden)
    if leaked:
        errors.append(f"credential metadata exposes secret material: {leaked}")
    if "revoked_at" not in properties:
        errors.append("credential metadata must expose revocation timestamp")
    if schema.get("additionalProperties") is not False:
        errors.append("credential metadata must reject undeclared fields")

    snippets = (
        "sha256.Sum256([]byte(secret))",
        "subtle.ConstantTimeCompare",
        "len([]byte(secret)) < minServiceSecretBytes",
        "func (c ServiceCredential) Rotate(",
        "func (c ServiceCredential) Revoke(",
        "func (c ServiceCredential) activeAt(",
    )
    for snippet in snippets:
        if snippet not in go_text:
            errors.append(f"credential lifecycle invariant missing: {snippet}")

    if "secretDigest" not in go_text:
        errors.append("credential digest must remain private Go state")
    if "SecretDigest" in go_text:
        errors.append("credential digest must not become exported state")
    if "security.AuthenticatedServicePrincipal" not in executor_text:
        errors.append("connector execution must require authenticated service principal")
    if "ErrConnectorCredentialDenied" not in executor_text:
        errors.append("connector credential denial semantics missing")
    return errors


def main() -> int:
    errors = validate(
        GO_CREDENTIAL.read_text(encoding="utf-8"),
        json.loads(SCHEMA.read_text(encoding="utf-8")),
        EXECUTOR.read_text(encoding="utf-8"),
    )
    if errors:
        print("SERVICE CREDENTIAL CONTRACT GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("SERVICE CREDENTIAL CONTRACT GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
