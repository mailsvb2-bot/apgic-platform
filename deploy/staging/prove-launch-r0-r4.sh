#!/usr/bin/env bash
# Fail-closed launch-cut gate: no test fixtures, mock provider or CI-only
# evidence may be substituted for completed staging operations.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"

ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
[[ -f "$ENV_FILE" ]] || { echo "LAUNCH BLOCKED: staging environment absent" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

: "${APGIC_COMMIT_SHA:?LAUNCH BLOCKED: deployed SHA absent}"
cd "$ROOT"
actual_sha="$(git rev-parse HEAD)"
if [[ "$actual_sha" != "$APGIC_COMMIT_SHA" ]]; then
  echo "LAUNCH BLOCKED: source/runtime SHA mismatch" >&2
  exit 1
fi
bash "$SCRIPT_DIR/check-staging-runtime.sh"

# A healthy HTTP service is not a completed non-custodial payment business.
# The current staging deployment explicitly disables R4 finance execution.
if [[ "${APGIC_FINANCE_WORKER_ENABLED:-0}" != "1" ||
      "${APGIC_OUTBOX_WORKER_ENABLED:-0}" != "1" ]]; then
  echo "LAUNCH BLOCKED: R4 finance and durable outbox workers must both be enabled" >&2
  exit 1
fi
for unit in apgic-finance-worker-staging.service apgic-outbox-worker-staging.service; do
  if ! systemctl is-active --quiet "$unit"; then
    echo "LAUNCH BLOCKED: $unit is not active" >&2
    exit 1
  fi
done

# Read-only proof of a real, already finished two-party consultation.
# Requires distinct client, specialist and stranger signed sessions and
# provider completion evidence. Never creates a booking or charges a card.
bash "$SCRIPT_DIR/prove-r0-r3-dual-party.sh"

# The evidence index and human-approved Canon dependencies must independently
# allow a production claim. No default manifest or silent PASS.
manifest="${APGIC_LAUNCH_EVIDENCE_MANIFEST:-}"
if [[ -z "$manifest" || ! -f "$ROOT/$manifest" ]]; then
  echo "LAUNCH BLOCKED: set APGIC_LAUNCH_EVIDENCE_MANIFEST to a repository evidence manifest" >&2
  exit 1
fi
python3 "$ROOT/tools/r4_release_gate.py" "$manifest" --gate all --mode production
printf 'APGIC R0-R4 LAUNCH GATE: PASS sha=%s\n' "$actual_sha"
