#!/usr/bin/env bash
set -euo pipefail

if [ "${EUID}" -ne 0 ]; then
  echo "run as root" >&2
  exit 1
fi

. /etc/os-release
if [ "${ID}" != "ubuntu" ] || [ "${VERSION_ID}" != "24.04" ]; then
  echo "APGIC host bootstrap requires Ubuntu 24.04" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y   nginx   postgresql   postgresql-contrib   git   curl   ca-certificates   ufw   certbot   python3-certbot-nginx \
  fail2ban

systemctl enable --now nginx postgresql

install -d -m 0755 /etc/fail2ban/jail.d
cat > /etc/fail2ban/jail.d/apgic-sshd.local <<'EOF'
[sshd]
enabled = true
backend = systemd
bantime = 1h
findtime = 10m
maxretry = 5
EOF
systemctl enable --now fail2ban

ufw allow OpenSSH
ufw allow "Nginx Full"
ufw --force enable

if ! swapon --show=NAME --noheadings | grep -qx "/swapfile"; then
  if [ ! -f /swapfile ]; then
    fallocate -l 2G /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
  fi
  swapon /swapfile
fi
if ! grep -qE '^/swapfile[[:space:]]+none[[:space:]]+swap[[:space:]]+sw[[:space:]]+0[[:space:]]+0$' /etc/fstab; then
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

install -d -m 0755 /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/apgic-limits.conf <<'EOF'
[Journal]
SystemMaxUse=200M
RuntimeMaxUse=100M
MaxRetentionSec=14day
EOF
systemctl restart systemd-journald

echo "APGIC dedicated host bootstrap: PASS"
