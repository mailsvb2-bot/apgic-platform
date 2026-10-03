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
OFFLINE_HOLD_ID=""
OFFLINE_IDEMPOTENCY_KEY="mobile-offline-e2e-ios"
COMPATIBILITY_BASE_URL="http://127.0.0.1:43113"
COMPATIBILITY_CONTRACT_VERSION="0.10.0-r0-remote-config"
REMOTE_CONFIG_E2E_KEY_ID="mobile027-e2e-key"
REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64="W2SJycf9Dc9QVF58FkiG70BJHsBsfxsSMEF5foEXU14="

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
  local status=$?
  trap - EXIT
  if [[ -n "$METRO_PID" ]]; then
    kill "$METRO_PID" 2>/dev/null || true
  fi
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$UDID" ]]; then
    xcrun simctl shutdown "$UDID" >/dev/null 2>&1 || true
  fi
  exit "$status"
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


prepare_offline_checkout() {
  local intent_id
  local slot_id
  local confirm_output="$EVIDENCE_DIR/ios-offline-confirm.json"
  local slots_output="$EVIDENCE_DIR/ios-offline-slots.json"
  local hold_output="$EVIDENCE_DIR/ios-offline-hold.json"

  intent_id="$(
    python3 - /tmp/apgic-installation-bootstrap-ios.json <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
value = payload.get("id", "")
if not value:
    raise SystemExit("bootstrap intent id missing")
print(value)
PY
  )"

  curl -fsS \
    -H "Cookie: $SESSION_COOKIE" \
    -H 'content-type: application/json' \
    --data '{"topics":["sleep"],"goals":[],"context":{}}' \
    "http://127.0.0.1:43113/v1/help-intents/$intent_id/confirm" \
    -o "$confirm_output"

  curl -fsS \
    -H "Cookie: $SESSION_COOKIE" \
    http://127.0.0.1:43113/v1/specialists/spec-lebedeva/slots \
    -o "$slots_output"

  slot_id="$(
    python3 - "$slots_output" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
slots = payload.get("slots", [])
if not slots:
    raise SystemExit("offline E2E slot missing")
print(slots[0]["id"])
PY
  )"

  curl -fsS \
    -H "Cookie: $SESSION_COOKIE" \
    -H 'content-type: application/json' \
    --data "{\"help_intent_id\":\"$intent_id\",\"slot_id\":\"$slot_id\"}" \
    http://127.0.0.1:43113/v1/slot-holds \
    -o "$hold_output"

  OFFLINE_HOLD_ID="$(
    python3 - "$hold_output" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
value = payload.get("id", "")
if not value:
    raise SystemExit("offline E2E hold id missing")
print(value)
PY
  )"
  [[ -n "$OFFLINE_HOLD_ID" ]] || fail "offline E2E hold was not created"
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
prepare_offline_checkout
issue_deep_link

export SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL=http://127.0.0.1:43113
export SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION=0.10.0-r0-remote-config

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

assert_compatibility_policy() {
  local supported_output="$EVIDENCE_DIR/ios-compatibility-supported.json"
  local update_output="$EVIDENCE_DIR/ios-compatibility-update-required.json"
  local supported_installation_id
  supported_installation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION=0.8.0-r2-offline-sync \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_SESSION_COOKIE="$SESSION_COOKIE" \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_ID="$supported_installation_id" \
  SIMCTL_CHILD_APGIC_E2E_INSTALLATION_PLATFORM=IOS \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$supported_output" 2>/dev/null &&
       json_has_ax_label "$supported_output" "installation-e2e:PASS" &&
       json_has_ax_label "$supported_output" "installation-e2e-state:REVOKED" &&
       json_has_ax_label "$supported_output" "installation-e2e-generation:2" &&
       ! json_has_ax_label "$supported_output" "compatibility-status:UPDATE_REQUIRED"; then
      echo "iOS supported previous contract remained operational: PASS"
      break
    fi
    sleep 1
  done
  json_has_ax_label "$supported_output" "installation-e2e:PASS" ||
    fail "supported previous iOS contract did not remain usable under the updated backend contract"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION=0.7.0-unsupported \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$update_output" 2>/dev/null &&
       json_has_ax_label "$update_output" "compatibility-e2e:PASS" &&
       json_has_ax_label "$update_output" "compatibility-status:UPDATE_REQUIRED" &&
       json_has_ax_label "$update_output" "compatibility-reason:CLIENT_CONTRACT_UNSUPPORTED" &&
       json_has_ax_label "$update_output" "compatibility-update-reason:INCOMPATIBLE_CRITICAL" &&
       json_has_ax_label "$update_output" "compatibility-update-action"; then
      echo "iOS installed-app compatibility + governed update path: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$update_output" ]] && cat "$update_output" >&2 || true
  fail "incompatible iOS client did not expose governed update-required UX"
}

assert_remote_config_kill_switch() {
  local network_output="$EVIDENCE_DIR/ios-remote-config-network.json"
  local cached_output="$EVIDENCE_DIR/ios-remote-config-last-known-safe.json"
  local consultation_id="mobile027-ios-kill-switch"
  local events="MICROPHONE_PERMISSION_GRANTED,NETWORK_TRANSPORT_CHANGED:CELLULAR,APP_FOREGROUND"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED   SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL="$COMPATIBILITY_BASE_URL"   SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION="$COMPATIBILITY_CONTRACT_VERSION"   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_BASE_URL=http://127.0.0.1:43113   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_KEY_ID="$REMOTE_CONFIG_E2E_KEY_ID"   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_PUBLIC_KEY_BASE64="$REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64"   SIMCTL_CHILD_APGIC_E2E_REALTIME_EVENTS="$events"   SIMCTL_CHILD_APGIC_E2E_REALTIME_CONSULTATION_ID="$consultation_id"     xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$network_output" 2>/dev/null &&
       json_has_ax_label "$network_output" "remote-config-e2e:PASS" &&
       json_has_ax_label "$network_output" "remote-config-source:NETWORK" &&
       json_has_ax_label "$network_output" "remote-config-reason:REMOTE_CONFIG_APPLIED" &&
       json_has_ax_label "$network_output" "remote-config-capability:REALTIME_CONSULTATION:DISABLED" &&
       ! json_has_ax_label "$network_output" "realtime-e2e:PASS"; then
      break
    fi
    sleep 1
  done

  json_has_ax_label "$network_output" "remote-config-source:NETWORK" ||
    fail "iOS did not apply signed remote config from canonical backend"
  ! json_has_ax_label "$network_output" "realtime-e2e:PASS" ||
    fail "iOS realtime ran despite remote kill switch"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED   SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL="$COMPATIBILITY_BASE_URL"   SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION="$COMPATIBILITY_CONTRACT_VERSION"   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_BASE_URL=http://127.0.0.1:43199   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_KEY_ID="$REMOTE_CONFIG_E2E_KEY_ID"   SIMCTL_CHILD_APGIC_E2E_REMOTE_CONFIG_PUBLIC_KEY_BASE64="$REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64"   SIMCTL_CHILD_APGIC_E2E_REALTIME_EVENTS="$events"   SIMCTL_CHILD_APGIC_E2E_REALTIME_CONSULTATION_ID="$consultation_id"     xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$cached_output" 2>/dev/null &&
       json_has_ax_label "$cached_output" "remote-config-e2e:PASS" &&
       json_has_ax_label "$cached_output" "remote-config-source:LAST_KNOWN_SAFE" &&
       json_has_ax_label "$cached_output" "remote-config-reason:REMOTE_CONFIG_FETCH_FAILED" &&
       json_has_ax_label "$cached_output" "remote-config-capability:REALTIME_CONSULTATION:DISABLED" &&
       ! json_has_ax_label "$cached_output" "realtime-e2e:PASS"; then
      echo "iOS signed remote config kill-switch + restart last-known-safe: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$cached_output" ]] && cat "$cached_output" >&2 || true
  fail "iOS did not preserve last-known-safe remote config across restart"
}

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

assert_offline_mutation_restart() {
  local pending_output="$EVIDENCE_DIR/ios-offline-mutation-pending.json"
  local confirmed_output="$EVIDENCE_DIR/ios-offline-mutation-confirmed.json"
  local replay_output="$EVIDENCE_DIR/ios-offline-mutation-server-replay.json"
  local side_effect

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE="$SESSION_COOKIE" \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_HOLD_ID="$OFFLINE_HOLD_ID" \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY="$OFFLINE_IDEMPOTENCY_KEY" \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_METHOD_CODE=BANK_CARD \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$pending_output" 2>/dev/null &&
       json_has_ax_label "$pending_output" "offline-mutation-e2e:LOCAL_PENDING" &&
       json_has_ax_label "$pending_output" "offline-mutation-attempts:1"; then
      break
    fi
    sleep 1
  done
  json_has_ax_label "$pending_output" "offline-mutation-e2e:LOCAL_PENDING" ||
    fail "offline checkout did not remain LOCAL_PENDING after committed response was lost"

  xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
  SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_BASE_URL=http://127.0.0.1:43113 \
  SIMCTL_CHILD_APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE="$SESSION_COOKIE" \
    xcrun simctl launch "$UDID" com.apgic.ci >/dev/null

  for _ in $(seq 1 60); do
    if "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$confirmed_output" 2>/dev/null &&
       json_has_ax_label "$confirmed_output" "offline-mutation-e2e:SERVER_CONFIRMED" &&
       json_has_ax_label "$confirmed_output" "offline-mutation-attempts:2"; then
      break
    fi
    sleep 1
  done
  json_has_ax_label "$confirmed_output" "offline-mutation-e2e:SERVER_CONFIRMED" ||
    fail "offline checkout did not recover from persisted queue after app restart"

  curl -fsS \
    -H "Cookie: $SESSION_COOKIE" \
    -H "Idempotency-Key: $OFFLINE_IDEMPOTENCY_KEY" \
    -H "X-Correlation-Id: offline:$OFFLINE_IDEMPOTENCY_KEY" \
    -H 'content-type: application/json' \
    --data "{\"hold_id\":\"$OFFLINE_HOLD_ID\",\"method_code\":\"BANK_CARD\"}" \
    http://127.0.0.1:43113/v1/mobile/checkout-instructions \
    -o "$replay_output"

  side_effect="$(
    python3 - "$replay_output" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("outcome") != "DUPLICATE_APPLIED":
    raise SystemExit(f"unexpected retry outcome: {payload!r}")
side_effect = payload.get("side_effect_ref", "")
checkout = payload.get("checkout") or {}
if not side_effect.startswith("checkout/") or side_effect != "checkout/" + checkout.get("id", ""):
    raise SystemExit(f"unexpected side effect: {payload!r}")
print(side_effect)
PY
  )"
  json_has_ax_label "$confirmed_output" "offline-mutation-side-effect:${side_effect}" ||
    fail "installed app and server replay disagree on canonical checkout side effect"

  echo "iOS installed-app offline checkout restart/retry: PASS"
}

assert_realtime_lifecycle() {
  local output="$EVIDENCE_DIR/ios-realtime-e2e.json"
  local rejoin_output="$EVIDENCE_DIR/ios-realtime-rejoin.json"
  local consultation_id="mobile009-ios-rejoin"
  local events="MICROPHONE_PERMISSION_REVOKED,NETWORK_OFFLINE,NETWORK_ONLINE,MICROPHONE_PERMISSION_GRANTED,NETWORK_TRANSPORT_CHANGED:CELLULAR,SCREEN_LOCKED,SCREEN_UNLOCKED,JOIN_AUTH_EXPIRED,APP_BACKGROUND,APP_FOREGROUND,AUDIO_ROUTE_CHANGED:BLUETOOTH,INTERRUPTION_BEGAN,NETWORK_ONLINE,INTERRUPTION_ENDED"

  xcrun simctl privacy "$UDID" grant microphone com.apgic.ci >/dev/null ||
    fail "failed to grant iOS microphone permission for realtime E2E"

  launch_realtime() {
    xcrun simctl terminate "$UDID" com.apgic.ci >/dev/null 2>&1 || true
    SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE=GRANTED     SIMCTL_CHILD_APGIC_E2E_REALTIME_EVENTS="$events"     SIMCTL_CHILD_APGIC_E2E_REALTIME_CONSULTATION_ID="$consultation_id"       xcrun simctl launch "$UDID" com.apgic.ci >/dev/null
  }

  realtime_ready() {
    local target="$1"
    "$IDB" ui describe-all --udid "$UDID" --api axbridge --json --nested >"$target" 2>/dev/null &&
      json_has_ax_label "$target" "realtime-e2e:PASS" &&
      json_has_ax_label "$target" "realtime-phase:CONNECTED" &&
      json_has_ax_label "$target" "realtime-business-transition:NONE" &&
      json_has_ax_label "$target" "realtime-audio-route:BLUETOOTH" &&
      json_has_ax_label "$target" "realtime-app-state:FOREGROUND" &&
      json_has_ax_label "$target" "realtime-network-state:ONLINE" &&
      json_has_ax_label "$target" "realtime-network-transport:CELLULAR" &&
      json_has_ax_label "$target" "realtime-screen-state:UNLOCKED" &&
      json_has_ax_label "$target" "realtime-join-auth-state:VALID" &&
      json_has_ax_label "$target" "realtime-consultation-id:${consultation_id}" &&
      json_has_ax_label "$target" "realtime-action-connect:true" &&
      json_has_ax_label "$target" "realtime-action-reconnect:true" &&
      json_has_ax_label "$target" "realtime-action-pause:true" &&
      json_has_ax_label "$target" "realtime-action-route:true" &&
      json_has_ax_label "$target" "realtime-action-auth:true"
  }

  ambient_technical_degraded() {
    local target="$1"
    [[ -f "$target" ]] &&
      json_has_ax_label "$target" "realtime-e2e:PASS" &&
      json_has_ax_label "$target" "realtime-phase:DEGRADED" &&
      json_has_ax_label "$target" "realtime-business-transition:NONE" &&
      json_has_ax_label "$target" "realtime-audio-route:BLUETOOTH" &&
      json_has_ax_label "$target" "realtime-app-state:FOREGROUND" &&
      json_has_ax_label "$target" "realtime-network-state:ONLINE" &&
      json_has_ax_label "$target" "realtime-network-transport:CELLULAR" &&
      json_has_ax_label "$target" "realtime-screen-state:UNLOCKED" &&
      json_has_ax_label "$target" "realtime-join-auth-state:VALID" &&
      json_has_ax_label "$target" "realtime-consultation-id:${consultation_id}" &&
      json_has_ax_label "$target" "realtime-action-connect:true" &&
      json_has_ax_label "$target" "realtime-action-reconnect:true" &&
      json_has_ax_label "$target" "realtime-action-pause:true" &&
      json_has_ax_label "$target" "realtime-action-route:true" &&
      json_has_ax_label "$target" "realtime-action-auth:true"
  }

  # Hosted iOS simulators can inject a real AVAudioSession interruption after
  # the scripted lifecycle reaches its terminal state. Do not accept DEGRADED
  # as success: retry the exact same canonical consultation only when every
  # other expected invariant is already proven, and keep CONNECTED mandatory.
  local first_pass=false
  local initial_attempt
  for initial_attempt in $(seq 1 3); do
    launch_realtime
    for _ in $(seq 1 60); do
      if realtime_ready "$output"; then
        first_pass=true
        break 2
      fi
      sleep 1
    done

    if ambient_technical_degraded "$output"; then
      echo "iOS initial realtime attempt $initial_attempt ended in ambient technical DEGRADED state; retrying same consultation"
      continue
    fi

    [[ -f "$output" ]] && cat "$output" >&2 || true
    fail "installed iOS app violated initial realtime lifecycle invariants"
  done
  if [[ "$first_pass" != "true" ]]; then
    [[ -f "$output" ]] && cat "$output" >&2 || true
    fail "installed iOS app did not complete native realtime lifecycle proof after bounded ambient-degradation retries"
  fi

  local rejoin_attempt
  for rejoin_attempt in $(seq 1 3); do
    launch_realtime
    for _ in $(seq 1 60); do
      if realtime_ready "$rejoin_output"; then
        echo "iOS installed-app native realtime lifecycle + restart/rejoin: PASS"
        return 0
      fi
      sleep 1
    done

    if ambient_technical_degraded "$rejoin_output"; then
      echo "iOS rejoin attempt $rejoin_attempt ended in ambient technical DEGRADED state; retrying same consultation"
      continue
    fi

    [[ -f "$rejoin_output" ]] && cat "$rejoin_output" >&2 || true
    fail "installed iOS app violated realtime restart/rejoin invariants"
  done

  [[ -f "$rejoin_output" ]] && cat "$rejoin_output" >&2 || true
  fail "installed iOS app did not rejoin the same canonical consultation after bounded restart retries"
}

assert_compatibility_policy
assert_remote_config_kill_switch
assert_installation_lifecycle
assert_deep_link_runtime
assert_notification_runtime
assert_offline_mutation_restart
assert_realtime_lifecycle

echo "IOS CAPABILITY + COMPATIBILITY + INSTALLATION + DEEP-LINK + NOTIFICATION + OFFLINE-SYNC + REALTIME NATIVE E2E: PASS"