#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "deploy/staging/check-staging-runtime.sh"
SERVICE = ROOT / "deploy/staging/apgic-staging-runtime-watchdog.service"
TIMER = ROOT / "deploy/staging/apgic-staging-runtime-watchdog.timer"
WORKER_PREFLIGHT = ROOT / "deploy/staging/check-staging-worker-units.sh"


def validate() -> list[str]:
    errors: list[str] = []
    script = SCRIPT.read_text(encoding="utf-8")
    service = SERVICE.read_text(encoding="utf-8")
    timer = TIMER.read_text(encoding="utf-8")
    worker_preflight = WORKER_PREFLIGHT.read_text(encoding="utf-8")
    updater = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")

    for item in (
        "/healthz",
        "/readyz",
        "/v1/meta",
        "APGIC_COMMIT_SHA",
        'APGIC_ENV_FILE:-/etc/apgic/staging.env',
        'source "$ENV_FILE"',
        "runtime SHA mismatch",
        'Host: ${APGIC_PUBLIC_HOST}',
        "--max-time 5",
        '--resolve "${APGIC_PUBLIC_HOST}:443:127.0.0.1"',
        '"https://${APGIC_PUBLIC_HOST}/"',
        'case "$http_status" in',
        'APGIC_OUTBOX_WORKER_ENABLED',
        'apgic-outbox-worker-staging.service',
        'APGIC_OUTBOX_MAX_PENDING_AGE_SECONDS',
        "delivery_status = 'PENDING'",
        'stale outbox backlog',
    ):
        if item not in script:
            errors.append(f"watchdog script missing invariant: {item}")

    for item in (
        'APGIC_FINANCE_WORKER_ENABLED',
        'apgic-finance-worker-staging.service',
        'APGIC_DATABASE_URL',
        'finance worker requires outbox delivery',
        'finance worker is not active',
        'financial schema is incomplete',
        "to_regclass('public.finance_settlement_instructions')",
        "to_regclass('public.finance_provider_evidence')",
        "to_regclass('public.finance_payout_decisions')",
        "to_regclass('public.finance_reconciliation_runs')",
        "to_regclass('public.finance_reconciliation_results')",
    ):
        if item not in script:
            errors.append(f"finance watchdog missing invariant: {item}")

    for item in (
        'finance_worker_enabled',
        'outbox_enabled',
        'APGIC finance worker activation requires APGIC_OUTBOX_WORKER_ENABLED=1',
        'APGIC_MFA_ENCRYPTION_KEY_BASE64',
        'APGIC staging deployment refused: missing independent AES-256 MFA key',
        'go build -o bin/apgic-finance-mfa-admin',
    ):
        if item not in updater:
            errors.append(f"staging deploy missing finance dependency: {item}")

    for item in (
        "DynamicUser=yes",
        "EnvironmentFile=/etc/apgic/staging.env",
        "ExecStart=/usr/bin/bash /opt/apgic/current/deploy/staging/check-staging-runtime.sh",
        "NoNewPrivileges=yes",
        "ProtectSystem=strict",
        "PrivateTmp=yes",
        "RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
    ):
        if item not in service:
            errors.append(f"watchdog service missing sandbox invariant: {item}")

    for item in (
        "ExecStartPre=+/usr/bin/bash /opt/apgic/current/deploy/staging/check-staging-worker-units.sh",
        "Environment=APGIC_WATCHDOG_UNIT_PREFLIGHT=systemd-preflight-v1",
    ):
        if item not in service:
            errors.append(f"watchdog worker preflight missing: {item}")
    for item in (
        "EUID != 0",
        "/usr/bin/systemctl --system is-active --quiet apgic-outbox-worker-staging.service",
        "/usr/bin/systemctl --system is-active --quiet apgic-finance-worker-staging.service",
        "finance worker requires outbox delivery",
    ):
        if item not in worker_preflight:
            errors.append(f"privileged worker preflight missing invariant: {item}")
    for item in ("worker_unit_active", "APGIC_WATCHDOG_UNIT_PREFLIGHT", "INVOCATION_ID"):
        if item not in script:
            errors.append(f"watchdog missing service-state handoff: {item}")

    if "OnBootSec=2m" not in timer or "OnUnitActiveSec=5m" not in timer:
        errors.append("watchdog timer cadence mismatch")
    if "apgic-staging-runtime-watchdog.service" not in timer:
        errors.append("watchdog timer unit target missing")

    combined = "\n".join((script, service, timer, worker_preflight)).lower()
    for forbidden in ("147.45.146.112", "metrotherapy", "default_server"):
        if forbidden in combined:
            errors.append(f"watchdog contains forbidden shared-host coupling: {forbidden}")

    return errors


def main() -> int:
    errors = validate()
    if errors:
        print("STAGING RUNTIME WATCHDOG GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("STAGING RUNTIME WATCHDOG GUARD: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
