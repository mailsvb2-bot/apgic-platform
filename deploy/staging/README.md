# APGIC staging boundary

This deployment profile is intentionally isolated from other products on a shared host.

- Repository/release root: /opt/apgic
- Secrets: /etc/apgic/staging.env
- API: 127.0.0.1:43111
- Web: 127.0.0.1:43112
- Temporary staging ingress: http://147.45.146.112 on a dedicated Host vhost
- PostgreSQL database/role: apgic_staging
- Services use systemd DynamicUser=yes; no persistent Unix application account is required.
- ProtectSystem=strict, NoNewPrivileges=yes, PrivateTmp=yes, and PrivateDevices=yes are mandatory.

The staging vhost may share port 80 only through server_name 147.45.146.112. It MUST NOT bind 443 or become default_server while Metrotherapy shares the host.
The committed environment file is a template only. Database credentials and commit SHA are written only during deployment.
This is staging/conformance evidence only and MUST NOT be represented as production approval or production release evidence.
