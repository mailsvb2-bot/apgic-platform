# R5 production account deletion truth gate

The existing `demand.Service.DeleteAccount` is an in-memory conformance
implementation. It fabricates the provider-erasure evidence string and
reports `profile_erased=true`; the PostgreSQL R5 personal profile and
other canonically owned customer data are not removed by this function.

**Safety rule:** staging/production must not claim account erasure through
this route. `BlockSyntheticAccountDeletion` is set by the API entrypoint
when a persistent database is required. The endpoint returns 503 with
`ACCOUNT_DELETION_EVIDENCE_UNAVAILABLE` and produces no deletion receipt.
Conformance-only isolated tests retain the old fixture behavior.

This is a truthful safety barrier, **not completed user account deletion**.
To reopen in production, implement:
1. durable, owner-signed, idempotent deletion request submission without
   prematurely claiming erasure;
2. identity re-confirmation and classification using an approved legal
   retention matrix;
3. scoped redaction/erasure of persisted preferences, check-ins and
   personal fields without mutating immutable financial and consultation
   evidence;
4. transactional outbox for independent provider erasure and verified
   receipts, retries and legal holds;
5. final public status only after independent completion evidence;
6. Web/iOS/Android actual device E2E verifying requests, retries,
   history visibility, session invalidation and retained financial records.

Until those proofs exist, the product should display deletion unavailable
and provide an appropriate support/legal request channel rather than
misrepresenting an API's receipt as a completed erasure.
