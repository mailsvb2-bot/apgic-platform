# APGIC staging boundary

## R0 foundation acceptance — CI versus real deployment

R0 requires trusted Identity → role and tenant permission → protected action →
durable PostgreSQL/audit/outbox result → replay/readback after restart, across
Web, iOS and Android. Passing CI cannot substitute for real MFA and installation.

On the **only authorized APGIC host** (92.51.23.254), after deploying the
exact candidate commit through the canonical updater, run:

```bash
sudo bash /opt/apgic/current/deploy/staging/prove-r0-foundation-readonly.sh
```

This read-only probe reuses existing host and watchdog protections, verifies
runtime SHA, canonical PostgreSQL tables, API/web services, backup/restore
timers and recent restore evidence. It does not mutate business data or
certify human authentication, tenant isolation, store builds or production.

The separate `tools/r0_production_admission.py` blocks promotion until
**all R0 canonical requirements** are VERIFIED/RELEASED and independently
reviewed exact-candidate evidence exists for multi-surface Identity, tenant
isolation, real human MFA/audit, staging, measured RPO/RTO, rollback and
signed iOS/Android installs. The production evidence index is currently
empty; readiness therefore remains BLOCKED. The config-only preflight
sets `production_promotion_eligible: false` by design.

Never relabel CI fixtures as operational or production evidence.

## Authorized host and Remote Desktop Commander boundary

The only authorized APGIC server is **IPv4 `92.51.23.254`**.

For APGIC work, Remote Desktop Commander MUST use only the connected device whose IPv4 is exactly `92.51.23.254`. It MUST NOT run diagnostics, inspection, cleanup, file operations, service commands, database commands, log reads, deployments, or any other APGIC command on another Desktop Commander device.

All server-mutating staging scripts source `deploy/staging/assert-authorized-host.sh` and fail closed when the local host does not own `92.51.23.254`.

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

## Mobile compatibility deployment gate

STAGING and PRODUCTION also require an explicit versioned mobile compatibility policy before the API starts. The policy includes AppVersion, BuildNumber, supported client contract revisions, the minimum/recommended app versions, a governed critical forced-update reason, and HTTPS update destinations for iOS and Android.

Before changing the checked-out commit, `update-staging.sh` validates these values from `/etc/apgic/staging.env`. Missing or malformed compatibility policy therefore refuses the deployment before `git reset`, migrations, or service restart. Use `deploy/staging/staging.env.example` as the canonical variable list.

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


## R4: real MFA and finance staging acceptance (release blocking)

**Unverified, not released.** Canon v7 APGIC-AUTH-002 requires actual higher-assurance
factor verification. The R4 operator path provides a server-verified TOTP
second factor and ten-minute session-bound step-up cookie; the factor is
provisioned out of band by a named administrator ONLY after the canonical
Identity received a finance-operator grant. This code is not a completed
human staging ceremony and cannot be used as production evidence until
real enrollment, test devices and audit proof have been independently reviewed.

Configure a 32-byte random AES key in the authorized host secret manager as
APGIC_MFA_ENCRYPTION_KEY_BASE64 (strict base64, separate from session signing).
After migration 000042 and grant provisioning, use the **private admin terminal**:

```bash
cd /opt/apgic/current/backend
go run ./cmd/finance-mfa-admin \
  --identity-id "<approved-identity-uuid>" \
  --approved-by "<named-approver>" \
  --approval-ref "<approved-change-ticket>"
```

The CLI outputs the authenticator secret **once**. Do not place it in shell
history, CI, ticket systems, chat, repository, telemetry or stdout captures.
Enroll it in the human operator's authenticator through an approved secure
channel. For high-risk finance actions, the operator enters a six-digit code
in the finance operations web view. The server checks active finance capability,
decrypts only on the backend, rejects replay, locks repeated failures and
emits immutable audit evidence; only success mints an HttpOnly Secure SameSite
step-up cookie for that exact browser session.

Do not expose a public self-issue endpoint, trust caller-provided identity,
generate a synthetic positive MFA assertion, or relabel green CI as a
human MFA success. Factor loss/rotation requires security-administrator review;
do not bypass the factor or edit economic truth as a recovery workaround.

### Mandatory human operator acceptance (not automatable with CI fixtures)

1. Provision a real finance operator using the existing append-only admin grant,
   with a named human and an out-of-band provisioned TOTP factor. Authenticate
   the same Identity and browser session; verify that attempting the high-risk
   discrepancy resolution WITHOUT fresh MFA returns HTTP 428 and leaves the
   discrepancy, ledger and audit business effects unchanged. Confirm the deny
   was audited.
2. Complete a REAL authenticator-code ceremony. A server-verified,
   one-time counter and session-bound assertion must be issued by APGIC
   only AFTER factor verification. A fresh authorized resolution must
   write exactly one append-only resolution and its actor/audit evidence.
3. Replay the same idempotency key and identical decision; require one economic
   effect and replay response. Mutate the decision while reusing the key;
   require a deterministic conflict. Expired / wrong-session MFA must return 428
   and create no resolution. Confirm no cross-tenant/operator data leakage.
4. On the authorized staging host, AFTER approved migration/deployment, run
   `sudo bash /opt/apgic/current/deploy/staging/prove-r4-finance-readonly.sh`.
   The probe requires active finance/outbox workers, durable finance tables,
   same runtime SHA and API health. Capture complete host-signed session logs
   and repository SHA as separate reviewed evidence. This read-only probe is
   *not* proof of real money movement or of successful MFA.
5. For every provider used for release, corroborate settlement, payout, refund
   and reconciliation with independently obtained PSP/bank/store evidence;
   synthetic CI events or database fixtures are invalid substitutes.

The production-only R4 payment gate requires `HUMAN_MFA_STEP_UP_E2E`,
`STAGING_FINANCE_END_TO_END`, `STAGING_FINANCE_WORKER_OUTBOX`, and
`STAGING_RECONCILIATION_DISCREPANCY_E2E`. These proof records must be
non-synthetic, bound to the exact candidate SHA, digested, reviewed and listed
in the immutable production evidence index. The v7 AUTH-002 registry status
must also be VERIFIED or RELEASED. Never change statuses merely to make CI pass.

The ordinary CI mode intentionally verifies gate mechanics with synthetic
fixtures; it cannot certify MFA enrollment, PSP payouts, or staging operations.

## R4 finance worker

`apgic-finance-worker-staging.service` is installed by the staging updater but is **disabled by default**. Set `APGIC_FINANCE_WORKER_ENABLED=1` in `/etc/apgic/staging.env` only after the R4 migrations are applied and the staging owner approves settlement/reconciliation processing; the updater then enables and restarts the worker after migration and runtime restart. The worker shares `APGIC_DATABASE_URL` and fails closed unless every R4 finance table is present. Enabling this service is not payment-provider certification or production payout evidence.

### R4 staging activation and proof

Finance worker and outbox delivery are one operational dependency: never enable finance alone. Configure APGIC_FINANCE_WORKER_ENABLED=1 only alongside APGIC_OUTBOX_WORKER_ENABLED=1 and a functioning authorized event gateway. The updater rejects misconfiguration before changing the checkout.

After deploying finance migrations, check both services and the runtime watchdog on the authorized staging host:

```bash
sudo systemctl is-active apgic-finance-worker-staging.service
sudo systemctl is-active apgic-outbox-worker-staging.service
sudo bash /opt/apgic/current/deploy/staging/check-staging-runtime.sh
sudo journalctl -u apgic-finance-worker-staging.service -n 100 --no-pager
```

The watchdog fails closed if finance is enabled but the worker is inactive, delivery is unavailable or the canonical finance schema is incomplete. It separately rejects stale pending outbox instructions. A healthy API is insufficient to claim financial readiness.

For a bank-free rehearsal, use the R4 PostgreSQL integration suite with APGIC_FINANCE_TEST_DATABASE_URL pointing to a disposable migrated test database. Its seeded evidence is synthetic and does not certify any PSP or money movement. Keep real provider signed proofs, payout statements, and real-device user journeys as external release gates.

## Transactional outbox worker

EVENT-001 uses a separate worker process rather than delivering external side effects inside API request handling. It is deliberately disabled unless a real compatible Event Gateway has been configured.

Set `APGIC_OUTBOX_WORKER_ENABLED=1` in `/etc/apgic/staging.env` only together with an HTTPS `APGIC_EVENT_GATEWAY_URL`, `APGIC_EVENT_GATEWAY_PRINCIPAL_ID`, and a minimum 32-byte `APGIC_EVENT_GATEWAY_CREDENTIAL`. Optional `APGIC_OUTBOX_BATCH_SIZE`, `APGIC_OUTBOX_POLL_INTERVAL_MS`, and `APGIC_OUTBOX_MAX_PENDING_AGE_SECONDS` tune delivery and watchdog thresholds. When disabled, staging must not be cited as EVENT-001 external-delivery evidence.

## Runtime watchdog

A systemd watchdog verifies the live staging runtime without using public DNS:

- checks API /healthz and /readyz over loopback;
- checks /v1/meta and requires commit_sha to equal APGIC_COMMIT_SHA from /etc/apgic/staging.env;
- checks the APGIC Nginx Host route over 127.0.0.1; after TLS cutover, an HTTP redirect is followed by a loopback HTTPS probe using the canonical hostname and certificate;
- starts two minutes after boot and repeats every five minutes with a small randomized delay;
- when the outbox worker is enabled, requires its systemd service to be active and rejects a stale `PENDING` outbox backlog;
- failures are fail-closed oneshot failures recorded in the system journal.

Enable apgic-staging-runtime-watchdog.timer after installing the service and timer units.


## GitHub-first deployment rule

GitHub is the source of truth for APGIC application code. Every deployed change must already exist in `mailsvb2-bot/apgic-platform` and be identified by a Git commit SHA before it reaches `92.51.23.254`.

Direct server edits are not an accepted development path. Generated artifacts, logs, caches, backups, and rollback binaries may exist on the host, but server-only source code is prohibited. If a useful change is discovered in a temporary worktree or server file, commit and push it to GitHub first, then deploy through the canonical updater.

## Migration-aware updates

Use `deploy/staging/update-staging.sh` for normal staging updates instead of manually applying individual migrations.

The updater:

- reads the currently deployed commit from `APGIC_COMMIT_SHA`;
- resolves the target commit (default: `origin/main`);
- validates the complete mobile compatibility policy before changing the checkout;
- refuses non-forward deployments;
- inspects numbered migration files added between the deployed and target commits;
- refuses modifications/deletions/renames of existing numbered migrations;
- builds API, the outbox worker, and Web before database changes;
- creates a PostgreSQL backup before applying newly added migrations;
- applies every newly added numbered migration in lexical order;
- updates `APGIC_COMMIT_SHA` only after builds and migrations succeed;
- restarts API/Web and requires the canonical runtime watchdog plus exact runtime SHA match.

Example:

```bash
cd /opt/apgic/current
bash deploy/staging/update-staging.sh origin/main
```

This path exists specifically to prevent code/schema skew when several migrations land between two deployed commits.


## Version-controlled EVENT-001 ingress

The canonical HTTPS gateway location is stored in
`deploy/staging/nginx-event-gateway-location.conf`. The installer
`deploy/staging/install-event-gateway-nginx.sh` writes that file to
`/etc/nginx/snippets/apgic-event-gateway.conf`, adopts only the previously
approved inline location in the Certbot TLS vhost, and replaces it with an
explicit `include`. It validates `nginx -t`, reloads Nginx, waits for
`HTTP 401` on a local unauthenticated HTTPS POST, verifies the website still
responds `200`, and restores both files on failure. Re-running the installer
updates the managed snippet without duplicate locations.

`update-staging.sh` reconciles this HTTPS route after building and restarting
API/Web **only when the gateway principal and credential are provisioned**;
otherwise it defers gateway activation. It starts the gateway receiver but
never enables the finance worker. Access remains restricted to
`127.0.0.1`/`::1` at Nginx, and the gateway independently requires a scoped
service bearer token. The outbox worker uses
`APGIC_EVENT_GATEWAY_URL=https://apgic.ru/_internal/apgic-event-gateway`
and `APGIC_EVENT_GATEWAY_DIAL_ADDR=127.0.0.1:443`. Do **not** change
`/etc/hosts` or disable TLS verification.

Secrets (`/etc/apgic/staging.env`), live PostgreSQL data, database backups,
Let's Encrypt private keys, Nginx host certificates, and service logs must
remain outside GitHub. GitHub versions the provisioning instructions and
scripts, not a copy of staging user data. A fresh host still requires
authorized host verification, TLS certificate issuance, private secrets and
real external event proof before production claims.
