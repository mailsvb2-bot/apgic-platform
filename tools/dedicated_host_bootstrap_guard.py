#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
BOOTSTRAP = ROOT / "deploy/staging/bootstrap-dedicated-host.sh"

def validate() -> list[str]:
    text = BOOTSTRAP.read_text(encoding="utf-8")
    errors: list[str] = []
    required = (
        'VERSION_ID}" != "24.04"',
        "nginx",
        "postgresql",
        "ufw",
        "certbot",
        "fail2ban",
        "apgic-sshd.local",
        "backend = systemd",
        "bantime = 1h",
        "findtime = 10m",
        "maxretry = 5",
        'ufw allow OpenSSH',
        'ufw allow "Nginx Full"',
        "fallocate -l 2G /swapfile",
        "SystemMaxUse=200M",
        "RuntimeMaxUse=100M",
        "MaxRetentionSec=14day",
        "systemctl enable --now nginx postgresql",
    )
    for item in required:
        if item not in text:
            errors.append(f"missing dedicated-host invariant: {item}")
    for forbidden in ("147.45.146.112", "metrotherapy", "default_server"):
        if forbidden.lower() in text.lower():
            errors.append(f"forbidden shared-host coupling: {forbidden}")
    return errors

def main() -> int:
    errors = validate()
    if errors:
        print("DEDICATED HOST BOOTSTRAP GUARD: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("DEDICATED HOST BOOTSTRAP GUARD: PASS")
    return 0

if __name__ == "__main__":
    sys.exit(main())
