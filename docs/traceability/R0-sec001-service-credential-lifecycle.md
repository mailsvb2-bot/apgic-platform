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

Keep APGIC-SEC-001 IN_PROGRESS until a deployed staging/service authentication probe proves active credential success plus expired/revoked credential denial without exposing credential material.
