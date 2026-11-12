#!/usr/bin/env bash
set -euo pipefail

# The watchdog's root-owned unit invokes this as ExecStartPre=+.
# Check only fixed, known service names; never start/stop any unit.
if (( EUID != 0 )); then
  echo "APGIC watchdog worker preflight requires privileged systemd context" >&2
  exit 1
fi
case "${APGIC_OUTBOX_WORKER_ENABLED:-0}" in
  0) ;;
  1)
    if ! /usr/bin/systemctl --system is-active --quiet apgic-outbox-worker-staging.service; then
      echo "APGIC watchdog worker preflight: outbox worker is not active" >&2
      exit 1
    fi
    ;;
  *) echo "APGIC_OUTBOX_WORKER_ENABLED must be 0 or 1" >&2; exit 1 ;;
esac
case "${APGIC_FINANCE_WORKER_ENABLED:-0}" in
  0) ;;
  1)
    if [[ "${APGIC_OUTBOX_WORKER_ENABLED:-0}" != "1" ]]; then
      echo "APGIC watchdog worker preflight: finance worker requires outbox delivery" >&2
      exit 1
    fi
    if ! /usr/bin/systemctl --system is-active --quiet apgic-finance-worker-staging.service; then
      echo "APGIC watchdog worker preflight: finance worker is not active" >&2
      exit 1
    fi
    ;;
  *) echo "APGIC_FINANCE_WORKER_ENABLED must be 0 or 1" >&2; exit 1 ;;
esac
