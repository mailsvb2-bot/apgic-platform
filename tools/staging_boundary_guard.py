#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
API = ROOT / "deploy/staging/apgic-api-staging.service"
WEB = ROOT / "deploy/staging/apgic-web-staging.service"
NGINX = ROOT / "deploy/staging/nginx-apgic-staging.conf"
ENV = ROOT / "deploy/staging/staging.env.example"

def validate() -> list[str]:
    errors = []
    api = API.read_text(encoding="utf-8")
    web = WEB.read_text(encoding="utf-8")
    nginx = NGINX.read_text(encoding="utf-8")
    env = ENV.read_text(encoding="utf-8")
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
    if "listen 18081;" not in nginx:
        errors.append("staging ingress must use isolated listener 18081")
    if "listen 80" in nginx or "listen 443" in nginx:
        errors.append("staging ingress must not take Metrotherapy ports 80/443")
    if "proxy_pass http://127.0.0.1:43112;" not in nginx:
        errors.append("staging ingress must proxy only to APGIC web loopback")
    if "APGIC_ENVIRONMENT=STAGING" not in env:
        errors.append("staging environment marker missing")
    if "APGIC_DATABASE_URL=REPLACED_AT_DEPLOY" not in env:
        errors.append("database secret must never be committed")
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
