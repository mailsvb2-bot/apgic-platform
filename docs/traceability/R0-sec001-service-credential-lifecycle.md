# APGIC-SEC-001 — service principal credential lifecycle

## Requirement

Production connectors/services authenticate with least-privilege service identities. Credential material must not enter repository, logs or client bundles, and credential rotation/revocation must be explicit and testable.

## Canonical execution path

1. `security.NewServicePrincipal` rejects empty and wildcard scopes.
2. `security.NewServiceCredential` accepts only high-entropy secrets and stores only a private SHA-256 digest.
3. `ServiceCredential.Authenticate` uses constant-time digest comparison and checks not-before/expiry/revocation.
4. `Rotate` revokes the prior credential version atomically in the returned state and creates a versioned successor.
5. `connector.Execute` accepts only `AuthenticatedServicePrincipal` and checks its credential validity before scope/provider execution.
6. `service-credential-metadata-v1.schema.json` serializes only non-secret lifecycle metadata.
7. `client_credential_guard.py` rejects server/service secret references in Web/React Native client source.
8. `repository_secret_scan.py` continues to block common committed secret formats across the repository.

## Failure behavior

- weak or malformed credential: `ErrInvalidServiceCredential`;
- future/expired/revoked credential: `ErrServiceCredentialDenied`;
- wrong secret: `ErrServiceSecretMismatch`;
- authenticated credential expired before connector execution: `ErrConnectorCredentialDenied`;
- valid credential without exact capability scope: `ErrConnectorScopeDenied`.

Provider execution is not called for any denied credential/scope path.

## Automated evidence

- `backend/internal/security/service_principal_test.go`
- `backend/internal/security/service_credential_test.go`
- `backend/internal/connector/executor_test.go`
- `tools/security_contract_guard.py`
- `tools/service_credential_contract_guard.py`
- `tools/client_credential_guard.py`
- `tools/tests/test_client_credential_guard.py`
- `tools/repository_secret_scan.py`
- `.github/workflows/ci.yml`

## Release status

APGIC-SEC-001 is VERIFIED. On 2026-10-05 candidate `e52e3b1246940e02835184c78b53c96d4bbc35d8` was executed on the authorized APGIC staging host as an isolated systemd transient service. `canon/evidence/staging-sec001-service-auth-20261005T195233Z.json` proves active credential success, wrong-secret/revoked/expired denial, exact-scope connector execution, wrong-scope denial, one provider call, and no emitted secret material.

## Current-main evidence binding

The staging proof remains applicable to current `main` because every SEC-001 implementation/contract/guard artifact below has the same Git blob SHA on the staging-tested candidate `e52e3b1246940e02835184c78b53c96d4bbc35d8` and on current main candidate `ad53cd5f03de159e8f5954cd6c540bed8e93be9c`:

- `backend/internal/security/service_principal.go` — `7b9e86d1722a9259c76d0d02d53123df04205dca`;
- `backend/internal/security/service_credential.go` — `ca11efdf10f027c9a4b968959c9be1927fa83646`;
- `backend/internal/connector/executor.go` — `9b619b7662d25a9fae34ee47ddab3ea8de8d366d`;
- `backend/cmd/service-auth-probe/main.go` — `3efedfd1138516d9bb8f80bc0ccbed78ccc9f5e2`;
- `contracts/jsonschema/service-credential-metadata-v1.schema.json` — `d1b23196153351ab5b2728e284f633faff38bbd6`;
- `contracts/jsonschema/service-principal-v1.schema.json` — `d04d1359d51659b7a6a27f49a4a275d9748f076b`;
- `tools/security_contract_guard.py` — `3df7d1acfa8a4015fd698ff89bf78deff4dae71f`;
- `tools/service_credential_contract_guard.py` — `9a5aa9979c556364090c38e85cce73c1b69d62a3`;
- `tools/client_credential_guard.py` — `e4c442f31b4f8ce7470ea75f7daf2478b645625e`;
- `tools/repository_secret_scan.py` — `c29a9ceb65e4bedd28daa0a7a1f8ee085eb73974`.

Current main CI run `37587880450` is green, including:

- Go format / vet / race tests — job `112682148207` — SUCCESS;
- Canon / architecture conformance — job `112682148163` — SUCCESS;
- R0 bootstrap gate — job `112689768024` — SUCCESS.

This promotes the Registry implementation status to `VERIFIED` against the required `DOMAIN_OR_CONTRACT_TEST` plus `E2E_OR_STAGING_PROOF`. It does not claim a production rollout or `RELEASED` status.

