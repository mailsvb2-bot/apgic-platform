#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="$ROOT/apps/mobile/ios/build/derived/Build/Products/Debug-iphonesimulator/APGIC.app"
EVIDENCE_DIR="$ROOT/evidence"
METRO_LOG="/tmp/apgic-metro-ios.log"
METRO_PID=""
SERVER_PID=""
UDID=""
SERVER_LOG="/tmp/apgic-mobile-installation-server-ios.log"
SESSION_COOKIE=""
DEEP_LINK_URL=""

fail() {
  echo "IOS CAPABILITY NATIVE E2E: FAIL: $*" >&2
  if [[ -f "$METRO_LOG" ]]; then
    tail -n 120 "$METRO_LOG" >&2 || true
  fi
  if [[ -f "$SERVER_LOG" ]]; then
    tail -n 120 "$SERVER_LOG" >&2 || true
  fi
  exit 1
}

cleanup() {
  if [[ -n "$METRO_PID" ]]; then
    kill "$METRO_PID" 2>/dev/null || true
  fi
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$UDID" ]]; then
    xcrun simctl shutdown "$UDID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

start_installation_server() {
  local binary="/tmp/apgic-mobile-installation-e2e-server"
  (
    cd "$ROOT/backend"
    go build -o "$binary" ./cmd/mobile-installation-e2e-server
  ) || fail "mobile installation E2E server build failed"
  APGIC_MOBILE_E2E_ADDR=127.0.0.1:43113 "$binary" >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!

  for _ in $(seq 1 60); do
    if curl -fsS http://127.0.0.1:43113/healthz >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      fail "mobile installation E2E server exited before becoming ready"
    fi
    sleep 1
  done
  fail "mobile installation E2E server did not become ready"
}

bootstrap_installation_session() {
  local headers
  headers="$(mktemp)"
  curl -fsS -D "$headers" -o /tmp/apgic-installation-bootstrap-ios.json \
    -H 'content-type: application/json' \
    --data '{"free_text":"ios native installation e2e"}' \
    http://127.0.0.1:43113/v1/help-intents >/dev/null
  SESSION_COOKIE="$(
    python3 - "$headers" <<'PY'
import sys
for raw in open(sys.argv[1], encoding="utf-8", errors="ignore"):
    if raw.lower().startswith("set-cookie:"):
        cookie = raw.split(":", 1)[1].strip().split(";", 1)[0]
        if cookie.startswith("__Host-apgic_session="):
            print(cookie)
            break
PY
  )"
  rm -f "$headers"
  [[ -n "$SESSION_COOKIE" ]] || fail "signed client session cookie was not issued"
}


issue_deep_link() {
  local output="$EVIDENCE_DIR/ios-deeplink-issued.json"
  curl -fsS     -H 'content-type: application/json'     --data '{"kind":"SPECIALIST","target_id":"e2e-specialist"}'     http://127.0.0.1:43113/v1/mobile/deep-links     -o "$output"
  DEEP_LINK_URL="$(
    python3 - "$output" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
url = payload.get("universal_url", "")
if not url.startswith("https://apgic.ru/l/v1."):
    raise SystemExit(f"unexpected universal_url: {url!r}")
print(url)
PY
  )"
  [[ -n "$DEEP_LINK_URL" ]] || fail "deep-link issue did not return canonical universal URL"
}

[[ -d "$APP" ]] || fail "simulator app missing: $APP"

xcrun simctl shutdown all >/dev/null 2>&1 || true
UDID="$(
  python3 - <<'PY'
import json
import subprocess

payload = json.loads(subprocess.check_output(
    ["xcrun", "simctl", "list", "devices", "available", "-j"],
    text=True,
))
candidates = []
for runtime, devices in payload.get("devices", {}).items():
    if "iOS" not in runtime:
        continue
    for device in devices:
        if device.get("isAvailable") and str(device.get("name", "")).startswith("iPhone"):
            candidates.append((runtime, device["name"], device["udid"]))
if not candidates:
    raise SystemExit("no available iPhone simulator")
candidates.sort(reverse=True)
print(candidates[0][2])
PY
)"
[[ -n "$UDID" ]] || fail "could not select iPhone simulator"

xcrun simctl boot "$UDID"
xcrun simctl bootstatus "$UDID" -b
xcrun simctl install "$UDID" "$APP"

(
  cd "$ROOT/apps/mobile"
  npx react-native start --port 8081 >"$METRO_LOG" 2>&1
) &
METRO_PID=$!

for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8081/status 2>/dev/null | grep -q "packager-status:running"; then
    break
  fi
  sleep 1
done
curl -fsS http://127.0.0.1:8081/status | grep -q "packager-status:running" ||
  fail "Metro did not become ready"
curl -fsS --max-time 120 "http://127.0.0.1:8081/index.bundle?platform=ios&dev=true&minify=false" \
  -o /tmp/apgic-ios-e2e.bundle ||
  fail "Metro iOS bundle did not become ready"

start_installation_server
bootstrap_installation_session
issue_deep_link

install_idb_with_retry() {
  if command -v idb >/dev/null 2>&1; then
    return 0
  fi

  brew tap facebook/fb
  for formula in idb idb-cli idb-companion; do
    brew trust --formula "facebook/fb/$formula"
  done

  local max_attempts=4
  local attempt
  for attempt in $(seq 1 "$max_attempts"); do
    echo "Installing idb (attempt $attempt/$max_attempts)..."
    if HOMEBREW_NO_AUTO_UPDATE=1 brew install facebook/fb/idb; then
      return 0
    fi
    if [[ "$attempt" -lt "$max_attempts" ]]; then
      # A transient GitHub Releases/Homebrew resource failure must not turn a
      # healthy installed-app E2E into a false product regression. Retry the
      # dependency download, but keep the E2E itself strictly fail-closed.
      sleep $((attempt * 10))
    fi
  done

  return 1
}

install_idb_with_retry || fail "idb CLI installation failed after bounded retries"
IDB="$(command -v idb || true)"
[[ -x "$IDB" ]] || fail "idb CLI was not installed"

mkdir -p "$EVIDENCE_DIR"

assert_state() {
  local state="$1"
  local reason="$2"
  local output="$EVIDENCE_DIR/ios-capability-e2e-${state}.json"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE="$state"     xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 30); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$output" 2>/dev/null &&
       grep -q "capability-state:${state}" "$output" &&
       grep -q "capability-fallback:${reason}" "$output"; then
      echo "iOS native E2E state $state: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "state $state did not expose expected fallback $reason"
}

assert_state "DENIED" "PERMISSION_DENIED"
assert_state "RESTRICTED" "OS_RESTRICTED"
assert_state "UNAVAILABLE" "CAPABILITY_UNAVAILABLE"

assert_installation_lifecycle() {
  local installation_id
  local output="$EVIDENCE_DIR/ios-installation-e2e.json"
  installation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_SESSION_COOKIE="$SESSION_COOKIE" \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_ID="$installation_id" \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_PLATFORM=IOS \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$output" 2>/dev/null &&
       grep -q 'installation-e2e:PASS' "$output" &&
       grep -q 'installation-e2e-state:REVOKED' "$output" &&
       grep -q 'installation-e2e-generation:2' "$output"; then
      curl -fsS -H "Cookie: $SESSION_COOKIE" http://127.0.0.1:43113/v1/mobile/installations \
        -o "$EVIDENCE_DIR/ios-installation-server-state.json"
      python3 - "$EVIDENCE_DIR/ios-installation-server-state.json" "$installation_id" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
installation_id = sys.argv[2]
matches = [item for item in payload.get("installations", []) if item.get("id") == installation_id]
if len(matches) != 1:
    raise SystemExit("expected exactly one installation record")
item = matches[0]
if item.get("state") != "REVOKED" or item.get("push_generation") != 2 or item.get("push_endpoint"):
    raise SystemExit(f"unexpected canonical installation state: {item!r}")
PY
      echo "iOS installed-app installation lifecycle: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed app did not complete register/rotate/revoke lifecycle"
}


json_has_ax_label() {
  local file="$1"
  local expected="$2"
  python3 - "$file" "$expected" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
expected = sys.argv[2]

def walk(value):
    if isinstance(value, dict):
        if value.get("AXLabel") == expected:
            return True
        return any(walk(item) for item in value.values())
    if isinstance(value, list):
        return any(walk(item) for item in value)
    return False

raise SystemExit(0 if walk(payload) else 1)
PY
}

assert_deep_link_runtime() {
  local output="$EVIDENCE_DIR/ios-deeplink-e2e.json"
  local expected_target="/specialists/e2e-specialist"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED   SIMCTL_CHILD_APGIC_E2E_DEEP_LINK_BASE_URL=http://127.0.0.1:43113   SIMCTL_CHILD_APGIC_E2E_DEEP_LINK_URL="$DEEP_LINK_URL"     xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$output" 2>/dev/null &&
       json_has_ax_label "$output" "deep-link-state:OPEN" &&
       json_has_ax_label "$output" "deep-link-target:${expected_target}"; then
      echo "iOS installed-app canonical deep-link resolution: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed iOS app did not resolve canonical deep link"
}

assert_notification_runtime() {
  local output="$EVIDENCE_DIR/ios-notification-e2e.json"
  local delivery_id="00000000-0000-0000-0000-00000000e701"
  local intent_id="00000000-0000-0000-0000-00000000e702"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_NOTIFICATION_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_NOTIFICATION_SESSION_COOKIE="$SESSION_COOKIE" \
  SIMCTL_CHILD_APGIC_E2E_NOTIFICATION_DELIVERY_ID="$delivery_id" \
  SIMCTL_CHILD_APGIC_E2E_NOTIFICATION_INTENT_ID="$intent_id" \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$output" 2>/dev/null &&
       json_has_ax_label "$output" "notification-e2e:PASS" &&
       json_has_ax_label "$output" "notification-e2e-intent:$intent_id" &&
       json_has_ax_label "$output" "notification-e2e-preview:GENERIC"; then
      echo "iOS installed-app canonical notification transport: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed iOS app did not resolve canonical notification transport"
}

assert_installation_lifecycle
assert_deep_link_runtime
assert_notification_runtime

echo "IOS CAPABILITY + INSTALLATION + DEEP-LINK + NOTIFICATION NATIVE E2E: PASS"
