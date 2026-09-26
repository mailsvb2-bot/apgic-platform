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
