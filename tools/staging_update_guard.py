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
        'git merge-base --is-ancestor',
        'git diff --name-status',
        "backend/migrations/[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql",
        'case "$status" in',
        'A)',
        'refusing deployment: existing migration changed',
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

    ordered(text, 'git merge-base --is-ancestor', 'git reset --hard "$TARGET_SHA"')
    ordered(text, 'go build -o bin/apgic-api ./cmd/api', 'systemctl start apgic-staging-backup.service')
    ordered(text, 'systemctl start apgic-staging-backup.service', '=== Reconcile migration ledger ===')
    ordered(text, '=== Reconcile migration ledger ===', 'APGIC_COMMIT_SHA=$TARGET_SHA')
    ordered(text, 'APGIC_COMMIT_SHA=$TARGET_SHA', 'systemctl restart apgic-api-staging.service')
    ordered(text, 'systemctl restart apgic-web-staging.service', 'wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1')
    ordered(text, 'wait_for_http "API" "http://127.0.0.1:43111/readyz" 30 1', 'wait_for_http "Web" "http://127.0.0.1:43112/" 30 1')
    ordered(text, 'wait_for_http "Web" "http://127.0.0.1:43112/" 30 1', 'bash "$REPO_ROOT/deploy/staging/check-staging-runtime.sh"')

    print("staging update guard: PASS")

if __name__ == "__main__":
    main()
