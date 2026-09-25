#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
export APGIC_HTTP_ADDR="${APGIC_HTTP_ADDR:-:43131}"
export APGIC_API_ORIGIN="${APGIC_API_ORIGIN:-http://127.0.0.1:43131}"
if ! command -v go >/dev/null 2>&1; then
  case "$(uname -m)" in
    x86_64) goarch=amd64 ;;
    aarch64|arm64) goarch=arm64 ;;
    *) echo "Go is not installed and $(uname -m) is unsupported" >&2; exit 1 ;;
  esac
  root="${TMPDIR:-/tmp}/apgic-go"
  mkdir -p "$root"
  curl -fsSL "https://go.dev/dl/go1.23.0.linux-${goarch}.tar.gz" | tar -C "$root" -xz
  export PATH="$root/go/bin:$PATH"
fi
cd "$ROOT/backend"
echo "building API with $(go version)"
go build -o /tmp/apgic-api ./cmd/api
/tmp/apgic-api > /tmp/apgic-api.log 2>&1 &
API_PID=$!
cleanup() { kill "$API_PID" 2>/dev/null || true; }
trap cleanup EXIT
ready=0
for _ in $(seq 1 30); do
  if curl -sf "${APGIC_API_ORIGIN}/healthz" >/dev/null; then
    ready=1
    break
  fi
  if ! kill -0 "$API_PID" 2>/dev/null; then
    break
  fi
  sleep 1
done
if [ "$ready" != 1 ]; then
  echo "API did not become ready at ${APGIC_API_ORIGIN}" >&2
  cat /tmp/apgic-api.log >&2 || true
  exit 1
fi
curl -sS -D - -o /dev/null "${APGIC_API_ORIGIN}/v1/slot-holds/missing/checkout-options?client_identity_id=x" | grep -qi 'content-type: application/json'
cd "$ROOT/apps/web"
exec npm run start -- --hostname 127.0.0.1 --port 43110
