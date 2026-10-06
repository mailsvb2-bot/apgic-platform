#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APK="$ROOT/apps/mobile/android/app/build/outputs/apk/debug/app-debug.apk"
EVIDENCE_DIR="$ROOT/evidence"
SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
AVD_NAME="apgic-native-e2e"
AVD_HOME="${ANDROID_AVD_HOME:-$HOME/.android/avd}"
SYSTEM_IMAGE="${APGIC_ANDROID_E2E_IMAGE:-system-images;android-36;google_apis;x86_64}"
METRO_LOG="/tmp/apgic-metro-android.log"
EMULATOR_LOG="/tmp/apgic-emulator.log"
METRO_PID=""
EMULATOR_PID=""
SERVER_PID=""
ADB=""
SERVER_LOG="/tmp/apgic-mobile-installation-server-android.log"
SESSION_COOKIE=""
DEEP_LINK_URL=""
OFFLINE_HOLD_ID=""
OFFLINE_IDEMPOTENCY_KEY="mobile-offline-e2e-android"
COMPATIBILITY_BASE_URL="http://127.0.0.1:43113"
COMPATIBILITY_CONTRACT_VERSION="0.10.0-r0-remote-config"
REMOTE_CONFIG_E2E_KEY_ID="mobile027-e2e-key"
REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64="W2SJycf9Dc9QVF58FkiG70BJHsBsfxsSMEF5foEXU14="

fail() {
  echo "ANDROID CAPABILITY NATIVE E2E: FAIL: $*" >&2
  if [[ -f "$METRO_LOG" ]]; then
    tail -n 120 "$METRO_LOG" >&2 || true
  fi
  if [[ -f "$EMULATOR_LOG" ]]; then
    tail -n 120 "$EMULATOR_LOG" >&2 || true
  fi
  if [[ -f "$SERVER_LOG" ]]; then
    tail -n 120 "$SERVER_LOG" >&2 || true
  fi
  exit 1
}

dump_until_labels_visible() {
  local remote="$1"
  local output="$2"
  shift 2
  local attempt
  local label
  local matched

  for attempt in $(seq 1 12); do
    if "$ADB" shell uiautomator dump "$remote" >/dev/null 2>&1 &&
       "$ADB" pull "$remote" "$output" >/dev/null 2>&1; then
      matched=true
      for label in "$@"; do
        if ! grep -Fq "$label" "$output"; then
          matched=false
          break
        fi
      done
      if [[ "$matched" == "true" ]]; then
        return 0
      fi
    fi

    # React Native renders long journeys in a ScrollView. Follow the same
    # vertical gesture a user would use instead of assuming all evidence is
    # present in the first 320x640 viewport.
    "$ADB" shell input swipe 160 540 160 180 250 >/dev/null 2>&1 || true
    sleep 1
  done
  return 1
}

tap_accessibility_label() {
  local label="$1"
  local remote="$2"
  local output="$3"
  local coords=""

  for _ in $(seq 1 20); do
    if "$ADB" shell uiautomator dump "$remote" >/dev/null 2>&1 &&
       "$ADB" pull "$remote" "$output" >/dev/null 2>&1; then
      coords="$(
        python3 - "$output" "$label" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET

path, label = sys.argv[1:]
root = ET.parse(path).getroot()
for node in root.iter("node"):
    if node.attrib.get("content-desc") != label:
        continue
    if node.attrib.get("enabled") != "true":
        continue
    match = re.fullmatch(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", node.attrib.get("bounds", ""))
    if not match:
        continue
    x1, y1, x2, y2 = map(int, match.groups())
    print(f"{(x1+x2)//2} {(y1+y2)//2}")
    break
PY
      )"
      if [[ -n "$coords" ]]; then
        # shellcheck disable=SC2086
        "$ADB" shell input tap $coords >/dev/null
        return 0
      fi
    fi
    "$ADB" shell input swipe 160 480 160 140 250 >/dev/null 2>&1 || true
    sleep 1
  done
  return 1
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
  if [[ -n "$ADB" ]]; then
    "$ADB" emu kill >/dev/null 2>&1 || true
  fi
  if [[ -n "$EMULATOR_PID" ]]; then
    kill "$EMULATOR_PID" 2>/dev/null || true
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
  curl -fsS -D "$headers" -o /tmp/apgic-installation-bootstrap-android.json \
    -H 'content-type: application/json' \
    --data '{"free_text":"android native installation e2e"}' \
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
  local confirm_output="$EVIDENCE_DIR/android-offline-confirm.json"
  local slots_output="$EVIDENCE_DIR/android-offline-slots.json"
  local hold_output="$EVIDENCE_DIR/android-offline-hold.json"

  intent_id="$(
    python3 - /tmp/apgic-installation-bootstrap-android.json <<'PY'
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
  local output="$EVIDENCE_DIR/android-deeplink-issued.json"
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

[[ -f "$APK" ]] || fail "debug APK missing: $APK"
[[ -n "$SDK_ROOT" ]] || fail "ANDROID_SDK_ROOT/ANDROID_HOME is not set"

SDKMANAGER="$SDK_ROOT/cmdline-tools/latest/bin/sdkmanager"
AVDMANAGER="$SDK_ROOT/cmdline-tools/latest/bin/avdmanager"
[[ -x "$SDKMANAGER" ]] || SDKMANAGER="$(command -v sdkmanager || true)"
[[ -x "$AVDMANAGER" ]] || AVDMANAGER="$(command -v avdmanager || true)"
[[ -x "$SDKMANAGER" ]] || fail "sdkmanager not found"
[[ -x "$AVDMANAGER" ]] || fail "avdmanager not found"

yes | "$SDKMANAGER" --licenses >/dev/null || true

install_android_sdk_packages() {
  local attempt
  for attempt in 1 2 3; do
    if "$SDKMANAGER" "platform-tools" "emulator" "$SYSTEM_IMAGE"; then
      return 0
    fi
    echo "Android SDK package install attempt $attempt failed; clearing transient cache before retry" >&2
    rm -rf "$HOME/.android/cache" >/dev/null 2>&1 || true
    sleep $((attempt * 2))
  done
  return 1
}

install_android_sdk_packages ||
  fail "Android SDK package installation failed after bounded retries"

ADB="$SDK_ROOT/platform-tools/adb"
[[ -x "$ADB" ]] || ADB="$(command -v adb || true)"
[[ -x "$ADB" ]] || fail "platform-tools installed but adb binary not found"

EMULATOR="$SDK_ROOT/emulator/emulator"
[[ -x "$EMULATOR" ]] || EMULATOR="$(command -v emulator || true)"
[[ -x "$EMULATOR" ]] || fail "emulator package installed but binary not found"

export ANDROID_AVD_HOME="$AVD_HOME"
mkdir -p "$ANDROID_AVD_HOME"
"$AVDMANAGER" delete avd -n "$AVD_NAME" >/dev/null 2>&1 || true
echo "no" | "$AVDMANAGER" create avd --force -n "$AVD_NAME" -k "$SYSTEM_IMAGE"
"$EMULATOR" -list-avds | grep -Fxq "$AVD_NAME" ||
  fail "AVD $AVD_NAME was created outside emulator search path $ANDROID_AVD_HOME"

if [[ -e /dev/kvm ]]; then
  sudo chmod 666 /dev/kvm
fi

"$EMULATOR"   -avd "$AVD_NAME"   -no-window   -no-audio   -no-snapshot   -no-boot-anim   -gpu swiftshader_indirect   >"$EMULATOR_LOG" 2>&1 &
EMULATOR_PID=$!

if ! timeout 120 "$ADB" wait-for-device; then
  fail "emulator did not become visible to adb within 120 seconds"
fi

for _ in $(seq 1 120); do
  if ! kill -0 "$EMULATOR_PID" 2>/dev/null; then
    fail "emulator process exited before boot completed"
  fi
  if [[ "$("$ADB" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]]; then
    break
  fi
  sleep 2
done
[[ "$("$ADB" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]] ||
  fail "emulator did not finish booting within 240 seconds"

"$ADB" shell settings put global window_animation_scale 0
"$ADB" shell settings put global transition_animation_scale 0
"$ADB" shell settings put global animator_duration_scale 0

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
curl -fsS --max-time 120 "http://127.0.0.1:8081/index.bundle?platform=android&dev=true&minify=false" \
  -o /tmp/apgic-android-e2e.bundle ||
  fail "Metro Android bundle did not become ready"

start_installation_server
bootstrap_installation_session
prepare_offline_checkout
issue_deep_link

"$ADB" reverse tcp:8081 tcp:8081
"$ADB" reverse tcp:43113 tcp:43113
"$ADB" install -r "$APK" >/dev/null
mkdir -p "$EVIDENCE_DIR"

assert_state() {
  local state="$1"
  local reason="$2"
  local remote="/sdcard/apgic-capability-${state}.xml"
  local local_file="$EVIDENCE_DIR/android-capability-e2e-${state}.xml"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W     -n com.apgic.ci/.MainActivity     --es APGIC_E2E_CAPABILITY_STATE "$state"     --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"     --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"     >/dev/null

  for _ in $(seq 1 30); do
    if "$ADB" shell uiautomator dump "$remote" >/dev/null 2>&1 &&
       "$ADB" pull "$remote" "$local_file" >/dev/null 2>&1 &&
       grep -q "capability-state:${state}" "$local_file" &&
       grep -q "capability-fallback:${reason}" "$local_file"; then
      echo "Android native E2E state $state: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$local_file" ]] && cat "$local_file" >&2 || true
  fail "state $state did not expose expected fallback $reason"
}

assert_state "DENIED" "PERMISSION_DENIED"
assert_state "RESTRICTED" "OS_RESTRICTED"
assert_state "UNAVAILABLE" "CAPABILITY_UNAVAILABLE"

assert_compatibility_policy() {
  local supported_output="$EVIDENCE_DIR/android-compatibility-supported.xml"
  local update_output="$EVIDENCE_DIR/android-compatibility-update-required.xml"
  local supported_installation_id
  supported_installation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_CONTRACT_VERSION 0.8.0-r2-offline-sync \
    --es APGIC_E2E_INSTALLATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_INSTALLATION_ID "$supported_installation_id" \
    --es APGIC_E2E_INSTALLATION_PLATFORM ANDROID \
    >/dev/null

  for _ in $(seq 1 60); do
    if dump_until_labels_visible /sdcard/apgic-compatibility-supported.xml "$supported_output"          'installation-e2e:PASS'          'installation-e2e-state:REVOKED'          'installation-e2e-generation:2' &&
       ! grep -q 'compatibility-status:UPDATE_REQUIRED' "$supported_output"; then
      echo "Android supported previous contract remained operational: PASS"
      break
    fi
    sleep 1
  done
  grep -q 'installation-e2e:PASS' "$supported_output" ||
    fail "supported previous Android contract did not remain usable under the updated backend contract"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_CONTRACT_VERSION 0.7.0-unsupported \
    >/dev/null

  for _ in $(seq 1 60); do
    if dump_until_labels_visible /sdcard/apgic-compatibility-update.xml "$update_output"          'compatibility-e2e:PASS'          'compatibility-status:UPDATE_REQUIRED'          'compatibility-reason:CLIENT_CONTRACT_UNSUPPORTED'          'compatibility-update-reason:INCOMPATIBLE_CRITICAL'          'compatibility-update-action'; then
      echo "Android installed-app compatibility + governed update path: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$update_output" ]] && cat "$update_output" >&2 || true
  fail "incompatible Android client did not expose governed update-required UX"
}

assert_remote_config_kill_switch() {
  local network_output="$EVIDENCE_DIR/android-remote-config-network.xml"
  local cached_output="$EVIDENCE_DIR/android-remote-config-last-known-safe.xml"
  local consultation_id="mobile027-android-kill-switch"
  local events="MICROPHONE_PERMISSION_GRANTED,NETWORK_TRANSPORT_CHANGED:CELLULAR,APP_FOREGROUND"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W     -n com.apgic.ci/.MainActivity     --es APGIC_E2E_CAPABILITY_STATE GRANTED     --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"     --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"     --es APGIC_E2E_REMOTE_CONFIG_BASE_URL http://127.0.0.1:43113     --es APGIC_E2E_REMOTE_CONFIG_KEY_ID "$REMOTE_CONFIG_E2E_KEY_ID"     --es APGIC_E2E_REMOTE_CONFIG_PUBLIC_KEY_BASE64 "$REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64"     --es APGIC_E2E_REALTIME_EVENTS "$events"     --es APGIC_E2E_REALTIME_CONSULTATION_ID "$consultation_id"     >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-remote-config-network.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-remote-config-network.xml "$network_output" >/dev/null 2>&1 &&
       grep -q 'remote-config-e2e:PASS' "$network_output" &&
       grep -q 'remote-config-source:NETWORK' "$network_output" &&
       grep -q 'remote-config-reason:REMOTE_CONFIG_APPLIED' "$network_output" &&
       grep -q 'remote-config-capability:REALTIME_CONSULTATION:DISABLED' "$network_output" &&
       ! grep -q 'realtime-e2e:PASS' "$network_output"; then
      break
    fi
    sleep 1
  done

  grep -q 'remote-config-source:NETWORK' "$network_output" ||
    fail "Android did not apply signed remote config from canonical backend"
  ! grep -q 'realtime-e2e:PASS' "$network_output" ||
    fail "Android realtime ran despite remote kill switch"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W     -n com.apgic.ci/.MainActivity     --es APGIC_E2E_CAPABILITY_STATE GRANTED     --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"     --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"     --es APGIC_E2E_REMOTE_CONFIG_BASE_URL http://127.0.0.1:43199     --es APGIC_E2E_REMOTE_CONFIG_KEY_ID "$REMOTE_CONFIG_E2E_KEY_ID"     --es APGIC_E2E_REMOTE_CONFIG_PUBLIC_KEY_BASE64 "$REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64"     --es APGIC_E2E_REALTIME_EVENTS "$events"     --es APGIC_E2E_REALTIME_CONSULTATION_ID "$consultation_id"     >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-remote-config-cache.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-remote-config-cache.xml "$cached_output" >/dev/null 2>&1 &&
       grep -q 'remote-config-e2e:PASS' "$cached_output" &&
       grep -q 'remote-config-source:LAST_KNOWN_SAFE' "$cached_output" &&
       grep -q 'remote-config-reason:REMOTE_CONFIG_FETCH_FAILED' "$cached_output" &&
       grep -q 'remote-config-capability:REALTIME_CONSULTATION:DISABLED' "$cached_output" &&
       ! grep -q 'realtime-e2e:PASS' "$cached_output"; then
      echo "Android signed remote config kill-switch + restart last-known-safe: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$cached_output" ]] && cat "$cached_output" >&2 || true
  fail "Android did not preserve last-known-safe remote config across restart"
}

assert_installation_lifecycle() {
  local installation_id
  local output="$EVIDENCE_DIR/android-installation-e2e.xml"
  installation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_INSTALLATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_INSTALLATION_ID "$installation_id" \
    --es APGIC_E2E_INSTALLATION_PLATFORM ANDROID \
    >/dev/null

  if dump_until_labels_visible /sdcard/apgic-installation-e2e.xml "$output" \
       'installation-e2e:PASS' \
       'installation-e2e-state:REVOKED' \
       'installation-e2e-generation:2'; then
    curl -fsS -H "Cookie: $SESSION_COOKIE" http://127.0.0.1:43113/v1/mobile/installations \
      -o "$EVIDENCE_DIR/android-installation-server-state.json"
    python3 - "$EVIDENCE_DIR/android-installation-server-state.json" "$installation_id" <<'PY'
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
    echo "Android installed-app installation lifecycle: PASS"
    return 0
  fi

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed app did not complete register/rotate/revoke lifecycle"
}


assert_deep_link_runtime() {
  local output="$EVIDENCE_DIR/android-deeplink-e2e.xml"
  local expected_target="/specialists/e2e-specialist"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W     -a android.intent.action.VIEW     -c android.intent.category.BROWSABLE     -d "$DEEP_LINK_URL"     -p com.apgic.ci     --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"     --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"     --es APGIC_E2E_DEEP_LINK_BASE_URL http://127.0.0.1:43113     >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-deeplink-e2e.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-deeplink-e2e.xml "$output" >/dev/null 2>&1 &&
       grep -q 'deep-link-state:OPEN' "$output" &&
       grep -q "deep-link-target:${expected_target}" "$output"; then
      echo "Android installed-app canonical deep-link VIEW intent: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android app did not resolve canonical deep-link VIEW intent"
}

assert_notification_runtime() {
  local output="$EVIDENCE_DIR/android-notification-e2e.xml"
  local delivery_id="00000000-0000-0000-0000-00000000e701"
  local intent_id="00000000-0000-0000-0000-00000000e702"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_NOTIFICATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_NOTIFICATION_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_NOTIFICATION_DELIVERY_ID "$delivery_id" \
    --es APGIC_E2E_NOTIFICATION_INTENT_ID "$intent_id" \
    >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-notification-e2e.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-notification-e2e.xml "$output" >/dev/null 2>&1 &&
       grep -q 'notification-e2e:PASS' "$output" &&
       grep -q "notification-e2e-intent:$intent_id" "$output" &&
       grep -q 'notification-e2e-preview:GENERIC' "$output"; then
      echo "Android installed-app canonical notification transport: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android app did not resolve canonical notification transport"
}

assert_offline_mutation_restart() {
  local pending_output="$EVIDENCE_DIR/android-offline-mutation-pending.xml"
  local confirmed_output="$EVIDENCE_DIR/android-offline-mutation-confirmed.xml"
  local replay_output="$EVIDENCE_DIR/android-offline-mutation-server-replay.json"
  local side_effect

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_OFFLINE_MUTATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_OFFLINE_MUTATION_HOLD_ID "$OFFLINE_HOLD_ID" \
    --es APGIC_E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY "$OFFLINE_IDEMPOTENCY_KEY" \
    --es APGIC_E2E_OFFLINE_MUTATION_METHOD_CODE BANK_CARD \
    >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-offline-pending.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-offline-pending.xml "$pending_output" >/dev/null 2>&1 &&
       grep -q 'offline-mutation-e2e:LOCAL_PENDING' "$pending_output" &&
       grep -q 'offline-mutation-attempts:1' "$pending_output"; then
      break
    fi
    sleep 1
  done
  grep -q 'offline-mutation-e2e:LOCAL_PENDING' "$pending_output" ||
    fail "offline checkout did not remain LOCAL_PENDING after committed response was lost"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_OFFLINE_MUTATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE "$SESSION_COOKIE" \
    >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-offline-confirmed.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-offline-confirmed.xml "$confirmed_output" >/dev/null 2>&1 &&
       grep -q 'offline-mutation-e2e:SERVER_CONFIRMED' "$confirmed_output" &&
       grep -q 'offline-mutation-attempts:2' "$confirmed_output" &&
       grep -q 'offline-mutation-side-effect:checkout/' "$confirmed_output"; then
      break
    fi
    sleep 1
  done
  grep -q 'offline-mutation-e2e:SERVER_CONFIRMED' "$confirmed_output" ||
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
  grep -q "offline-mutation-side-effect:${side_effect}" "$confirmed_output" ||
    fail "installed app and server replay disagree on canonical checkout side effect"

  echo "Android installed-app offline checkout restart/retry: PASS"
}

assert_workspace_switch() {
  local output="$EVIDENCE_DIR/android-workspace-e2e.xml"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_WORKSPACE_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_WORKSPACE_SESSION_COOKIE "$SESSION_COOKIE" \
    >/dev/null

  if dump_until_labels_visible /sdcard/apgic-workspace-e2e.xml "$output" \
       'workspace-e2e:PASS' \
       'workspace-e2e-kinds:CLIENT|SPECIALIST|ORGANIZATION' \
       'workspace-e2e-foreign-denied:true'; then
    echo "Android installed-app one-Identity multi-role workspace switching: PASS"
    return 0
  fi

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android app did not prove CLIENT/SPECIALIST/ORGANIZATION workspace switching"
}

assert_realtime_lifecycle() {
  local output="$EVIDENCE_DIR/android-realtime-e2e.xml"
  local rejoin_output="$EVIDENCE_DIR/android-realtime-rejoin.xml"
  local consultation_id="mobile009-android-rejoin"
  local events="MICROPHONE_PERMISSION_REVOKED,NETWORK_OFFLINE,NETWORK_ONLINE,MICROPHONE_PERMISSION_GRANTED,NETWORK_TRANSPORT_CHANGED:CELLULAR,SCREEN_LOCKED,SCREEN_UNLOCKED,JOIN_AUTH_EXPIRED,APP_BACKGROUND,APP_FOREGROUND,INTERRUPTION_BEGAN,NETWORK_ONLINE,INTERRUPTION_ENDED,AUDIO_ROUTE_CHANGED:BLUETOOTH"

  "$ADB" shell pm grant com.apgic.ci android.permission.RECORD_AUDIO >/dev/null ||
    fail "failed to grant Android microphone permission for realtime E2E"

  launch_realtime() {
    "$ADB" shell am force-stop com.apgic.ci
    "$ADB" shell am start -W       -n com.apgic.ci/.MainActivity       --es APGIC_E2E_CAPABILITY_STATE GRANTED       --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"       --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"       --es APGIC_E2E_REALTIME_EVENTS "$events"       --es APGIC_E2E_REALTIME_CONSULTATION_ID "$consultation_id"       >/dev/null
  }

  realtime_ready() {
    local target="$1"
    local viewport="${target}.viewport.xml"
    local attempt
    local phase

    : >"$target"
    for attempt in $(seq 1 60); do
      if "$ADB" shell uiautomator dump /sdcard/apgic-realtime-e2e.xml >/dev/null 2>&1 &&
         "$ADB" pull /sdcard/apgic-realtime-e2e.xml "$viewport" >/dev/null 2>&1; then
        cat "$viewport" >>"$target"
        if grep -q 'realtime-e2e:PASS' "$target" &&
           grep -q 'realtime-phase:CONNECTED' "$target" &&
           grep -q 'realtime-business-transition:NONE' "$target" &&
           grep -Eq 'realtime-audio-route:(SPEAKER|EARPIECE|BLUETOOTH|WIRED|UNKNOWN)' "$target" &&
           grep -Eq 'realtime-audio-route-observed:[^"]*BLUETOOTH' "$target" &&
           grep -q 'realtime-app-state:FOREGROUND' "$target" &&
           grep -q 'realtime-network-state:ONLINE' "$target" &&
           grep -Eq 'realtime-network-transport:(WIFI|CELLULAR|ETHERNET|OTHER|UNKNOWN)' "$target" &&
           grep -Eq 'realtime-network-transport-observed:[^"]*CELLULAR' "$target" &&
           grep -q 'realtime-screen-state:UNLOCKED' "$target" &&
           grep -q 'realtime-join-auth-state:VALID' "$target" &&
           grep -q "realtime-consultation-id:${consultation_id}" "$target" &&
           grep -q 'realtime-action-connect:true' "$target" &&
           grep -q 'realtime-action-reconnect:true' "$target" &&
           grep -q 'realtime-action-pause:true' "$target" &&
           grep -q 'realtime-action-route:true' "$target" &&
           grep -Eq 'realtime-provider-actions:[^"]*REFRESH_JOIN_AUTH' "$target"; then
          rm -f "$viewport"
          return 0
        fi
      fi

      # Sweep the ScrollView down and back up so evidence that spans more than
      # one emulator viewport is collected without assuming a fixed screen size.
      phase=$(( (attempt - 1) % 16 ))
      if (( phase < 8 )); then
        "$ADB" shell input swipe 160 540 160 180 250 >/dev/null 2>&1 || true
      else
        "$ADB" shell input swipe 160 180 160 540 250 >/dev/null 2>&1 || true
      fi
      sleep 1
    done
    rm -f "$viewport"
    return 1
  }

  launch_realtime
  if ! realtime_ready "$output"; then
    [[ -f "$output" ]] && tail -n 40 "$output" >&2 || true
    fail "installed Android app did not complete native realtime lifecycle proof"
  fi

  launch_realtime
  if realtime_ready "$rejoin_output"; then
    echo "Android installed-app native realtime lifecycle + restart/rejoin: PASS"
    return 0
  fi

  [[ -f "$rejoin_output" ]] && tail -n 40 "$rejoin_output" >&2 || true
  fail "installed Android app did not rejoin the same canonical consultation after process restart"
}

assert_accessibility_and_device_matrix() {
  local output="$EVIDENCE_DIR/android-accessibility-e2e.xml"
  local matrix="$EVIDENCE_DIR/android-device-matrix.json"
  local density
  local target_px

  density="$("$ADB" shell wm density | sed -n 's/.*Physical density: //p' | tr -d '\r' | tail -n1)"
  if [[ -z "$density" ]]; then
    density="$("$ADB" shell wm density | grep -Eo '[0-9]+' | head -n1)"
  fi
  [[ -n "$density" ]] || fail "could not determine Android display density"
  target_px="$(python3 - "$density" <<'PY'
import math
import sys
print(math.ceil(44 * int(sys.argv[1]) / 160))
PY
)"

  "$ADB" shell settings put system font_scale 1.30
  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --ez APGIC_E2E_ACCESSIBILITY true \
    >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-accessibility.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-accessibility.xml "$output" >/dev/null 2>&1 &&
       grep -q 'a11y-action-primary' "$output" &&
       grep -q 'a11y-action-secondary' "$output" &&
       grep -q 'a11y-font-scale:' "$output" &&
       grep -q 'a11y-reduced-motion:' "$output"; then
      break
    fi
    sleep 1
  done

  python3 - "$output" "$target_px" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET

path = sys.argv[1]
target_px = int(sys.argv[2])
root = ET.parse(path).getroot()
nodes = list(root.iter("node"))

def find(label):
    for node in nodes:
        if node.attrib.get("content-desc") == label:
            return node
    raise SystemExit(f"missing accessibility node {label}")

primary = find("a11y-action-primary")
secondary = find("a11y-action-secondary")
for label, node in (("primary", primary), ("secondary", secondary)):
    if node.attrib.get("clickable") != "true":
        raise SystemExit(f"{label} action is not clickable")
    if node.attrib.get("enabled") != "true":
        raise SystemExit(f"{label} action is not enabled")
    bounds = node.attrib.get("bounds", "")
    match = re.fullmatch(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", bounds)
    if not match:
        raise SystemExit(f"{label} action bounds missing: {bounds!r}")
    x1, y1, x2, y2 = map(int, match.groups())
    if x2 - x1 < target_px or y2 - y1 < target_px:
        raise SystemExit(
            f"{label} action below 44dp touch target: {(x2-x1)}x{(y2-y1)} px < {target_px}px"
        )

labels = [node.attrib.get("content-desc") for node in nodes]
if labels.index("a11y-action-primary") >= labels.index("a11y-action-secondary"):
    raise SystemExit("critical action accessibility order is not deterministic")
PY

  python3 - "$ADB" "$EVIDENCE_DIR" "$matrix" <<'PY'
import json
import pathlib
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

adb, evidence_dir, output = sys.argv[1:]
profiles = [
    ("PHONE_COMPACT", "720x1280", "320"),
    ("PHONE_LARGE", "1080x2400", "420"),
    ("TABLET", "1600x2560", "320"),
]
result = []
for name, size, density in profiles:
    subprocess.run([adb, "shell", "wm", "size", size], check=True, stdout=subprocess.DEVNULL)
    subprocess.run([adb, "shell", "wm", "density", density], check=True, stdout=subprocess.DEVNULL)
    subprocess.run([adb, "shell", "am", "force-stop", "com.apgic.ci"], check=True, stdout=subprocess.DEVNULL)
    subprocess.run(
        [adb, "shell", "am", "start", "-W", "-n", "com.apgic.ci/.MainActivity",
         "--es", "APGIC_E2E_CAPABILITY_STATE", "GRANTED",
         "--ez", "APGIC_E2E_ACCESSIBILITY", "true"],
        check=True,
        stdout=subprocess.DEVNULL,
    )
    remote = f"/sdcard/apgic-a11y-{name.lower()}.xml"
    local = pathlib.Path(evidence_dir) / f"android-a11y-{name.lower()}.xml"
    ready = False
    for _ in range(45):
        if subprocess.run([adb, "shell", "uiautomator", "dump", remote], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            if subprocess.run([adb, "pull", remote, str(local)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                root = ET.parse(local).getroot()
                labels = [n.attrib.get("content-desc") for n in root.iter("node")]
                if "a11y-action-primary" in labels and "a11y-action-secondary" in labels:
                    ready = True
                    break
        time.sleep(1)
    if not ready:
        raise SystemExit(f"{name}: accessibility controls not operable at representative size")
    result.append({"device_class": name, "size_px": size, "density_dpi": int(density), "status": "PASS"})

pathlib.Path(output).write_text(json.dumps({"version": 1, "profiles": result}, indent=2) + "\n", encoding="utf-8")
PY

  "$ADB" shell wm size reset >/dev/null
  "$ADB" shell wm density reset >/dev/null
  "$ADB" shell settings put system font_scale 1.0
  echo "Android accessibility semantics + text scaling + device matrix: PASS"
}

assert_help_intent_confirmation() {
  local output="$EVIDENCE_DIR/android-demand-e2e.xml"
  local remote="/sdcard/apgic-demand-e2e.xml"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_DEMAND_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_DEMAND_SESSION_COOKIE "$SESSION_COOKIE" \
    >/dev/null

  tap_accessibility_label "С чем нужна помощь" "$remote" "$output" ||
    fail "Android production HelpIntent input was not operable"
  "$ADB" shell input text 'anxiety%ssleep' >/dev/null
  "$ADB" shell input keyevent 4 >/dev/null
  sleep 1

  tap_accessibility_label "Разобрать запрос" "$remote" "$output" ||
    fail "Android production HelpIntent analyze action was not operable"

  if ! dump_until_labels_visible "$remote" "$output" \
       'native-demand-diagnosis:false' \
       'Тема anxiety' \
       'Тема sleep'; then
    [[ -f "$output" ]] && cat "$output" >&2 || true
    fail "Android production HelpIntent interpretation/no-diagnosis state was not visible"
  fi

  tap_accessibility_label "Тема anxiety" "$remote" "$output" ||
    fail "Android production HelpIntent suggested topic could not be corrected"
  tap_accessibility_label "Подтвердить темы запроса" "$remote" "$output" ||
    fail "Android production HelpIntent confirmation action was not operable"

  if dump_until_labels_visible "$remote" "$output" \
       'native-demand-confirmed' \
       'native-demand-diagnosis:false' \
       'Тема sleep' &&
     ! grep -Fq 'Тема anxiety' "$output"; then
    echo "Android installed-app production HelpIntent interpretation/correction/no-diagnosis: PASS"
    return 0
  fi

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android production UI did not prove HelpIntent correction/no-diagnosis flow"
}

assert_tenant_isolation() {
  local output="$EVIDENCE_DIR/android-capability-e2e-auth001.xml"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_AUTHZ_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_AUTHZ_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_AUTHZ_OWN_ORGANIZATION_ID 00000000-0000-0000-0000-00000000a001 \
    --es APGIC_E2E_AUTHZ_FOREIGN_ORGANIZATION_ID 00000000-0000-0000-0000-00000000b001 \
    --es APGIC_E2E_AUTHZ_FOREIGN_PRIVATE_MARKER "AUTH001_FOREIGN_PRIVATE_SENTINEL_7F4A9C" \
    --es APGIC_E2E_AUTHZ_SURFACE ANDROID \
    >/dev/null

  if dump_until_labels_visible /sdcard/apgic-auth001-e2e.xml "$output" \
       'authz-e2e:PASS' \
       'authz-e2e-own-allowed:true' \
       'authz-e2e-cross-denied:true' \
       'authz-e2e-forged-denied:true' \
       'authz-e2e-disclosure-blocked:true' \
       'authz-e2e-cross-audit:true' \
       'authz-e2e-forged-audit:true'; then
    if grep -Fq 'AUTH001_FOREIGN_PRIVATE_SENTINEL_7F4A9C' "$output"; then
      fail "installed Android AUTH-001 proof disclosed foreign private marker"
    fi
    echo "Android installed-app AUTH-001 tenant isolation + audit: PASS"
    return 0
  fi

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android app did not prove AUTH-001 tenant isolation"
}

assert_account_deletion() {
  local output="$EVIDENCE_DIR/android-deletion-e2e.xml"
  local identity_id
  local request_id="android-deletion-e2e-request"

  identity_id="$(
    python3 - /tmp/apgic-installation-bootstrap-android.json <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
value = payload.get("client_identity_id", "")
if not value:
    raise SystemExit("bootstrap identity id missing")
print(value)
PY
  )"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL" \
    --es APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION" \
    --es APGIC_E2E_DELETION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_DELETION_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_DELETION_IDENTITY_ID "$identity_id" \
    --es APGIC_E2E_DELETION_REQUEST_ID "$request_id" \
    --es APGIC_E2E_DELETION_PLATFORM ANDROID \
    >/dev/null

  if dump_until_labels_visible /sdcard/apgic-deletion-e2e.xml "$output" \
       'deletion-e2e:PASS' \
       'deletion-e2e-state:PARTIALLY_RETAINED_WITH_REASON' \
       'deletion-e2e-deactivation:false' \
       'deletion-e2e-profile-erased:true' \
       'deletion-e2e-ledger-retained:true' \
       'deletion-e2e-idempotent:true'; then
    echo "Android installed-app canonical account deletion + idempotent replay: PASS"
    return 0
  fi

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed Android app did not complete canonical account deletion"
}

assert_compatibility_policy
assert_remote_config_kill_switch
assert_installation_lifecycle
assert_workspace_switch
assert_tenant_isolation
assert_deep_link_runtime
assert_notification_runtime
assert_offline_mutation_restart
assert_realtime_lifecycle
assert_accessibility_and_device_matrix
assert_help_intent_confirmation
assert_account_deletion

echo "ANDROID CAPABILITY + COMPATIBILITY + INSTALLATION + AUTHZ + DEMAND + DELETION + DEEP-LINK + NOTIFICATION + OFFLINE-SYNC + REALTIME + ACCESSIBILITY NATIVE E2E: PASS"
