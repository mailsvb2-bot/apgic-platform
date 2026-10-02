#!/usr/bin/env python3
from pathlib import Path

SCRIPT = Path("deploy/staging/update-staging.sh")

def require(text: str, needle: str) -> None:
    if needle not in text:
        raise SystemExit(f"staging updater contract missing: {needle}")

def ordered(text: str, first: str, second: str) -> None:
    if text.find(first) == -1 or text.find(second) == -1 or text.find(first) >= text.find(second):
        raise SystemExit(f"staging updater ordering invalid: {first!r} must precede {second!r}")

def main() -> None:
    text = SCRIPT.read_text(encoding="utf-8")

    for needle in (
        'APGIC_COMMIT_SHA',
        'APGIC_DEPLOY_LOCK_FILE',
        'APGIC_MOBILE_POLICY_VERSION',
        'APGIC_MOBILE_CONTRACT_VERSION',
        'APGIC_MOBILE_SUPPORTED_CONTRACTS',
        'APGIC_MOBILE_FORCED_UPDATE_REASON',
        'APGIC_IOS_MIN_BUILD',
        'APGIC_ANDROID_MIN_BUILD',
        'validate_semver()',
        'validate_positive_integer()',
        'validate_https_url()',
        'current mobile contract is not in APGIC_MOBILE_SUPPORTED_CONTRACTS',
        'flock -n 9',
        'another APGIC staging deployment is already running',
        'git merge-base --is-ancestor',
        'git diff --name-status',
        "backend/migrations/[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql",
        'case "$status" in',
        'A)',
        'refusing deployment: existing migration changed',
        "SELECT to_regclass('public.apgic_schema_migrations') IS NOT NULL",
        'backup_required=false',
        '[[ "$ledger_exists" != "t" ]] || (("${#new_migrations[@]}" > 0))',
        '=== Backup PostgreSQL before migration ledger bootstrap ===',
        '=== Backup PostgreSQL before new migrations ===',
        'systemctl start apgic-staging-backup.service',
        'refusing deployment: managed migration contains transaction control',
        'deploy/staging/apply-staging-migrations.sh',
        '=== Reconcile migration ledger ===',
        'go build -o bin/apgic-api ./cmd/api',
        'npm run build',
        'systemctl restart apgic-api-staging.service',
        'systemctl restart apgic-web-staging.service',
        'wait_for_http()',
        'wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1',
        'wait_for_http "Web" "http://127.0.0.1:43112/" 30 1',
        'journalctl -u apgic-api-staging.service -n 80 --no-pager',
        'journalctl -u apgic-web-staging.service -n 80 --no-pager',
        'bash "$REPO_ROOT/deploy/staging/check-staging-runtime.sh"',
        'runtime SHA mismatch',
    ):
        require(text, needle)

    ordered(text, 'flock -n 9', 'APGIC_MOBILE_POLICY_VERSION is required')
    ordered(text, 'APGIC_MOBILE_POLICY_VERSION is required', 'git fetch origin main')
    ordered(text, 'git fetch origin main', 'git merge-base --is-ancestor')
    ordered(text, 'git merge-base --is-ancestor', 'git reset --hard "$TARGET_SHA"')
    ordered(text, 'go build -o bin/apgic-api ./cmd/api', "SELECT to_regclass('public.apgic_schema_migrations') IS NOT NULL")
    ordered(text, "SELECT to_regclass('public.apgic_schema_migrations') IS NOT NULL", 'systemctl start apgic-staging-backup.service')
    ordered(text, 'systemctl start apgic-staging-backup.service', '=== Reconcile migration ledger ===')
    ordered(text, '=== Reconcile migration ledger ===', 'APGIC_COMMIT_SHA=$TARGET_SHA')
    ordered(text, 'APGIC_COMMIT_SHA=$TARGET_SHA', 'systemctl restart apgic-api-staging.service')
    ordered(text, 'systemctl restart apgic-web-staging.service', 'wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1')
    ordered(text, 'wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1', 'wait_for_http "Web" "http://127.0.0.1:43112/" 30 1')
    ordered(text, 'wait_for_http "Web" "http://127.0.0.1:43112/" 30 1', 'bash "$REPO_ROOT/deploy/staging/check-staging-runtime.sh"')

    print("staging update guard: PASS")

if __name__ == "__main__":
    main()
