# APGIC staging boundary

This deployment profile targets a dedicated APGIC VPS. No other product is expected to share its runtime, database, or ingress configuration.

- Repository/release root: /opt/apgic
- Secrets: /etc/apgic/staging.env
- API: 127.0.0.1:43111
- Web: 127.0.0.1:43112
- Public ingress: apgic.ru and www.apgic.ru through Nginx
- PostgreSQL database/role: apgic_staging
- Services use systemd DynamicUser=yes; no persistent Unix application account is required.
- ProtectSystem=strict, NoNewPrivileges=yes, PrivateTmp=yes, and PrivateDevices=yes are mandatory.

The application API, Web runtime, and PostgreSQL MUST remain loopback-only. Only Nginx owns public HTTP/HTTPS ingress.
The committed environment file is a template only. Database credentials and commit SHA are written only during deployment.
This is staging/conformance evidence only and MUST NOT be represented as production approval or production release evidence.

## Domain and TLS cutover

The canonical public domain is apgic.ru with www.apgic.ru as an alias.
DNS must resolve to the dedicated APGIC VPS before ACME certificate issuance.
HTTP may be used as a bootstrap ingress before DNS propagation. After certificate issuance, Nginx should own 443 for apgic.ru/www.apgic.ru and redirect HTTP to HTTPS.

## Runtime database gate

STAGING and PRODUCTION runtime modes require APGIC_DATABASE_URL.
The API verifies PostgreSQL connectivity and canonical tables before startup and rechecks storage from /readyz.
An unavailable or incomplete database must make the service fail closed; staging must never silently fall back to in-memory readiness.

## Operational PostgreSQL backups

Staging uses an operational backup path that is intentionally separate from CI restore evidence and production release evidence.

- Daily custom-format logical backup: 02:15 UTC, with up to 10 minutes randomized delay.
- Backup files live under /var/backups/apgic, owned by postgres, directory mode 0700 and dump mode 0600.
- Backup creation is atomic: a hidden temporary dump is validated with pg_restore --list before rename.
- Default retention is 7 days.
- Weekly restore verification: Sunday 03:30 UTC into a temporary apgic_restore_verify_* database.
- Verification compares source/restored public-table counts and checks critical R0 tables, then drops the temporary database.
- The live apgic_staging database is never a restore target.

Install the four systemd units/timers from deploy/staging, run systemctl daemon-reload, then enable:
- apgic-staging-backup.timer
- apgic-staging-restore-verify.timer

Timeweb VM snapshots complement this logical backup path; they do not replace logical restore verification.


## Runtime watchdog

A systemd watchdog verifies the live staging runtime without using public DNS:

- checks API /healthz and /readyz over loopback;
- checks /v1/meta and requires commit_sha to equal APGIC_COMMIT_SHA from /etc/apgic/staging.env;
- checks the APGIC Nginx Host route over 127.0.0.1;
- starts two minutes after boot and repeats every five minutes with a small randomized delay;
- failures are fail-closed oneshot failures recorded in the system journal.

Enable apgic-staging-runtime-watchdog.timer after installing the service and timer units.
