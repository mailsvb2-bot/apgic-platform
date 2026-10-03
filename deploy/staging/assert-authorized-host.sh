#!/usr/bin/env bash
set -euo pipefail

AUTHORIZED_APGIC_IPV4="92.51.23.254"

if ! command -v ip >/dev/null 2>&1; then
  echo "APGIC host boundary: cannot verify IPv4 because 'ip' is unavailable" >&2
  exit 78
fi

if ! ip -4 -o addr show scope global | awk '{print $4}' | cut -d/ -f1 | grep -Fxq "$AUTHORIZED_APGIC_IPV4"; then
  echo "APGIC host boundary violation: this host is not $AUTHORIZED_APGIC_IPV4" >&2
  echo "APGIC server-side work is authorized only on IPv4 $AUTHORIZED_APGIC_IPV4" >&2
  exit 78
fi

printf 'APGIC host boundary: PASS ipv4=%s\n' "$AUTHORIZED_APGIC_IPV4"
