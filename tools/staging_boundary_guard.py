#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
API = ROOT / "deploy/staging/apgic-api-staging.service"
WEB = ROOT / "deploy/staging/apgic-web-staging.service"
NGINX = ROOT / "deploy/staging/nginx-apgic-staging.conf"
ENV = ROOT / "deploy/staging/staging.env.example"
HOST_GUARD = ROOT / "deploy/staging/assert-authorized-host.sh"
AGENTS = ROOT / "AGENTS.md"
SERVER_SCRIPTS = [
    ROOT / "deploy/staging/update-staging.sh",
    ROOT / "deploy/staging/check-staging-runtime.sh",
    ROOT / "deploy/staging/bootstrap-dedicated-host.sh",
    ROOT / "deploy/staging/backup-staging-postgres.sh",
    ROOT / "deploy/staging/verify-staging-backup.sh",
]
AUTHORIZED_IPV4 = "92.51.23.254"

def validate() -> list[str]:
    errors = []
    api = API.read_text(encoding="utf-8")
    web = WEB.read_text(encoding="utf-8")
    nginx = NGINX.read_text(encoding="utf-8")
    env = ENV.read_text(encoding="utf-8")
    host_guard = HOST_GUARD.read_text(encoding="utf-8")
    agents = AGENTS.read_text(encoding="utf-8")
    for name, text in (("api", api), ("web", web)):
        for required in ("DynamicUser=yes", "NoNewPrivileges=yes", "ProtectSystem=strict", "PrivateTmp=yes"):
            if required not in text:
                errors.append(f"{name}: missing sandbox invariant {required}")
        if "EnvironmentFile=/etc/apgic/staging.env" not in text:
            errors.append(f"{name}: staging environment file boundary missing")
    if "127.0.0.1:43111" not in env or "APGIC_API_ORIGIN=http://127.0.0.1:43111" not in env:
        errors.append("API must remain loopback-only on 43111")
    if "--hostname 127.0.0.1 --port 43112" not in web:
        errors.append("Web must remain loopback-only on 43112")
    if "listen 80;" not in nginx or "server_name apgic.ru www.apgic.ru;" not in nginx:
        errors.append("staging ingress must use the dedicated APGIC domain host on port 80")
    if "default_server" in nginx:
        errors.append("APGIC ingress must never become an implicit default-server")
    if "proxy_pass http://127.0.0.1:43112;" not in nginx:
        errors.append("staging ingress must proxy only to APGIC web loopback")
    if "APGIC_ENVIRONMENT=STAGING" not in env:
        errors.append("staging environment marker missing")
    if "APGIC_DATABASE_URL=REPLACED_AT_DEPLOY" not in env:
        errors.append("database secret must never be committed")
    if AUTHORIZED_IPV4 not in host_guard:
        errors.append(f"host guard must pin authorized APGIC IPv4 {AUTHORIZED_IPV4}")
    if AUTHORIZED_IPV4 not in agents or "Remote Desktop Commander hard rule" not in agents:
        errors.append("AGENTS.md must document the APGIC Remote Desktop Commander host boundary")
    if "GitHub-first source of truth" not in agents:
        errors.append("AGENTS.md must document GitHub-first APGIC source of truth")
    for script in SERVER_SCRIPTS:
        text = script.read_text(encoding="utf-8")
        if "assert-authorized-host.sh" not in text:
            errors.append(f"{script.relative_to(ROOT)}: missing fail-closed authorized-host guard")
    return errors

def main() -> int:
    errors = validate()
    if errors:
        print("STAGING BOUNDARY GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("STAGING BOUNDARY GUARD: PASS")
    return 0

if __name__ == "__main__":
    sys.exit(main())
