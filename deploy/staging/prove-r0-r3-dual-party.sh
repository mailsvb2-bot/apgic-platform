#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/assert-authorized-host.sh"
ENV_FILE="${APGIC_ENV_FILE:-/etc/apgic/staging.env}"
[[ -f "$ENV_FILE" ]] || { echo "R0-R3 dual party: staging environment absent" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
: "${APGIC_COMMIT_SHA:?}"
: "${APGIC_R0_R3_ACCEPTANCE_BOOKING_ID:?}"
: "${APGIC_R0_R3_ACCEPTANCE_SESSION_COOKIE:?}"
: "${APGIC_R0_R3_SPECIALIST_SESSION_COOKIE:?}"
: "${APGIC_R0_R3_STRANGER_SESSION_COOKIE:?}"
bash "$SCRIPT_DIR/check-staging-runtime.sh"
python3 "$SCRIPT_DIR/../../tools/r0_r3_dual_party_readback.py" \
 --api-base "http://${APGIC_HTTP_ADDR:-127.0.0.1:43111}" \
 --candidate-sha "$APGIC_COMMIT_SHA" \
 --booking-id "$APGIC_R0_R3_ACCEPTANCE_BOOKING_ID"
