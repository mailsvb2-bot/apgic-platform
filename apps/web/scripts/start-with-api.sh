#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
export APGIC_WEB_PORT="${APGIC_WEB_PORT:-43110}"
if [ -z "${APGIC_HTTP_ADDR:-}" ]; then
  if [ -n "${CI:-}" ]; then
    API_PORT="$(APGIC_WEB_PORT="$APGIC_WEB_PORT" node - <<'NODE'
const net = require("node:net");
const forbiddenPort = Number(process.env.APGIC_WEB_PORT || "0");

function choosePort() {
  const server = net.createServer();
  server.listen(0, "127.0.0.1", () => {
    const address = server.address();
    if (!address || typeof address === "string") {
      process.exitCode = 1;
      server.close();
      return;
    }
    const port = address.port;
    server.close(() => {
      if (port === forbiddenPort) {
        choosePort();
        return;
      }
      process.stdout.write(String(port));
    });
  });
  server.on("error", () => {
    process.exitCode = 1;
  });
}

choosePort();
NODE
)"
    export APGIC_HTTP_ADDR=":${API_PORT}"
    export APGIC_API_ORIGIN="http://127.0.0.1:${API_PORT}"
  else
    export APGIC_HTTP_ADDR=":43131"
    export APGIC_API_ORIGIN="${APGIC_API_ORIGIN:-http://127.0.0.1:43131}"
  fi
else
  api_port="${APGIC_HTTP_ADDR##*:}"
  export APGIC_API_ORIGIN="${APGIC_API_ORIGIN:-http://127.0.0.1:${api_port}}"
fi
export APGIC_CLIENT_SESSION_KEY="${APGIC_CLIENT_SESSION_KEY:-local-only-client-session-key-000000000000}"
# Browser conformance tests use synthetic provider events only in an isolated local API.
export APGIC_CONFORMANCE_PROVIDER_EVENTS=1
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
WEB_PID=""
cleanup() {
  if [ -n "$WEB_PID" ]; then
    kill "$WEB_PID" 2>/dev/null || true
  fi
  kill "$API_PID" 2>/dev/null || true
  if [ -n "$WEB_PID" ]; then
    wait "$WEB_PID" 2>/dev/null || true
  fi
  wait "$API_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
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
npm run start -- --hostname 127.0.0.1 --port "$APGIC_WEB_PORT" &
WEB_PID=$!
wait "$WEB_PID"
