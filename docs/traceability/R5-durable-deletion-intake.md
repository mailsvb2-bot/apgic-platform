# R5 durable deletion intake

The existing PostgreSQL `delete_account_requests` is canonical. The new authenticated `POST /v1/client/account-deletion-requests` accepts only `WEB`, `IOS`, `ANDROID` and records a persistent `REQUESTED` row. `GET` reads its status for the same signed owner. Both responses are non-cacheable. Duplicate submissions for the same identity are serialized with a transaction-scoped advisory lock and return the same persisted request.

This is **not** erasure completion and makes no such claim. The existing synthetic `POST /v1/account-deletions` remains blocked in production. Execution of confirmed deletion, second-factor reconfirmation, legal retention and provider receipts is still unimplemented, so this release must not be described as complete account deletion.
