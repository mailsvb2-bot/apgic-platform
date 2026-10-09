# APGIC-NFR-001 — runtime SLI telemetry

## Requirement

Critical paths must expose measurable SLI/SLO signals, dependency failure reasons and alertable runtime evidence before production readiness.

## Runtime path

1. Every API request is observed after canonical Go ServeMux routing.
2. Metrics use only HTTP method, canonical route pattern and status code. Raw URLs, query parameters, booking IDs, identity IDs and payload content are not exported.
3. Request latency is exposed as a Prometheus-compatible histogram with fixed millisecond buckets.
4. Readiness decisions are recorded separately by result and stable reason code so storage/config dependency failures are distinguishable from generic HTTP errors.
5. `/metrics` exposes the current in-process SLI snapshot.
6. The staging runtime check calls `/readyz`, then requires HTTP request, latency histogram and readiness-series evidence from `/metrics`.

## Fail-closed / privacy behavior

- Unmatched paths are labelled `UNMATCHED`; the raw path is never used as a metric label.
- Readiness configuration failures remain HTTP 503 and are counted with `CONFIG_REQUIRED`.
- Storage readiness failures remain HTTP 503 and are counted with `STORAGE_UNAVAILABLE`.
- This implementation does not invent production SLO thresholds or alert budgets. Those remain governed by the production LaunchConfig/SLO policy.

## Automated evidence

- `backend/internal/httpapi/handler_test.go` proves status/latency/readiness metrics and that raw path identifiers are not leaked into metric labels.
- `deploy/staging/check-staging-runtime.sh` proves the deployed runtime emits the required SLI families after an actual readiness check.
- `.github/workflows/ci.yml` executes Go race tests and staging shell/guard validation.

## Release status

APGIC-NFR-001 remains `IN_PROGRESS` until production-approved SLO/RPO/RTO values and deployed production/staging alert evidence exist. This code closes the missing runtime telemetry path but does not fabricate production operations evidence.
