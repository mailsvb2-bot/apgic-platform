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

cleanup() {
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
"$SDKMANAGER" "platform-tools" "emulator" "$SYSTEM_IMAGE"

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
  "$ADB" shell am start -W     -n com.apgic.ci/.MainActivity     --es APGIC_E2E_CAPABILITY_STATE "$state"     >/dev/null

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

assert_installation_lifecycle() {
  local installation_id
  local output="$EVIDENCE_DIR/android-installation-e2e.xml"
  installation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W \
    -n com.apgic.ci/.MainActivity \
    --es APGIC_E2E_CAPABILITY_STATE GRANTED \
    --es APGIC_E2E_INSTALLATION_BASE_URL http://127.0.0.1:43113 \
    --es APGIC_E2E_SESSION_COOKIE "$SESSION_COOKIE" \
    --es APGIC_E2E_INSTALLATION_ID "$installation_id" \
    --es APGIC_E2E_INSTALLATION_PLATFORM ANDROID \
    >/dev/null

  for _ in $(seq 1 60); do
    if "$ADB" shell uiautomator dump /sdcard/apgic-installation-e2e.xml >/dev/null 2>&1 &&
       "$ADB" pull /sdcard/apgic-installation-e2e.xml "$output" >/dev/null 2>&1 &&
       grep -q 'installation-e2e:PASS' "$output" &&
       grep -q 'installation-e2e-state:REVOKED' "$output" &&
       grep -q 'installation-e2e-generation:2' "$output"; then
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
    sleep 1
  done

  [[ -f "$output" ]] && cat "$output" >&2 || true
  fail "installed app did not complete register/rotate/revoke lifecycle"
}


assert_deep_link_runtime() {
  local output="$EVIDENCE_DIR/android-deeplink-e2e.xml"
  local expected_target="/specialists/e2e-specialist"

  "$ADB" shell am force-stop com.apgic.ci
  "$ADB" shell am start -W     -a android.intent.action.VIEW     -c android.intent.category.BROWSABLE     -d "$DEEP_LINK_URL"     -p com.apgic.ci     --es APGIC_E2E_DEEP_LINK_BASE_URL http://127.0.0.1:43113     >/dev/null

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

assert_realtime_lifecycle() {
  local output="$EVIDENCE_DIR/android-realtime-e2e.xml"
  local rejoin_output="$EVIDENCE_DIR/android-realtime-rejoin.xml"
  local consultation_id="mobile009-android-rejoin"
  local events="MICROPHONE_PERMISSION_REVOKED,NETWORK_OFFLINE,NETWORK_ONLINE,MICROPHONE_PERMISSION_GRANTED,NETWORK_TRANSPORT_CHANGED:CELLULAR,SCREEN_LOCKED,SCREEN_UNLOCKED,JOIN_AUTH_EXPIRED,APP_BACKGROUND,APP_FOREGROUND,AUDIO_ROUTE_CHANGED:BLUETOOTH,INTERRUPTION_BEGAN,NETWORK_ONLINE,INTERRUPTION_ENDED"

  "$ADB" shell pm grant com.apgic.ci android.permission.RECORD_AUDIO >/dev/null ||
    fail "failed to grant Android microphone permission for realtime E2E"

  launch_realtime() {
    "$ADB" shell am force-stop com.apgic.ci
    "$ADB" shell am start -W       -n com.apgic.ci/.MainActivity       --es APGIC_E2E_CAPABILITY_STATE GRANTED       --es APGIC_E2E_REALTIME_EVENTS "$events"       --es APGIC_E2E_REALTIME_CONSULTATION_ID "$consultation_id"       >/dev/null
  }

  realtime_ready() {
    local target="$1"
    "$ADB" shell uiautomator dump /sdcard/apgic-realtime-e2e.xml >/dev/null 2>&1 &&
      "$ADB" pull /sdcard/apgic-realtime-e2e.xml "$target" >/dev/null 2>&1 &&
      grep -q 'realtime-e2e:PASS' "$target" &&
      grep -q 'realtime-phase:CONNECTED' "$target" &&
      grep -q 'realtime-business-transition:NONE' "$target" &&
      grep -q 'realtime-audio-route:BLUETOOTH' "$target" &&
      grep -q 'realtime-app-state:FOREGROUND' "$target" &&
      grep -q 'realtime-network-state:ONLINE' "$target" &&
      grep -q 'realtime-network-transport:CELLULAR' "$target" &&
      grep -q 'realtime-screen-state:UNLOCKED' "$target" &&
      grep -q 'realtime-join-auth-state:VALID' "$target" &&
      grep -q "realtime-consultation-id:${consultation_id}" "$target" &&
      grep -q 'realtime-action-connect:true' "$target" &&
      grep -q 'realtime-action-reconnect:true' "$target" &&
      grep -q 'realtime-action-pause:true' "$target" &&
      grep -q 'realtime-action-route:true' "$target" &&
      grep -Eq 'realtime-provider-actions:[^"]*REFRESH_JOIN_AUTH' "$target"
  }

  launch_realtime
  local first_pass=false
  for _ in $(seq 1 60); do
    if realtime_ready "$output"; then
      first_pass=true
      break
    fi
    sleep 1
  done
  if [[ "$first_pass" != "true" ]]; then
    [[ -f "$output" ]] && cat "$output" >&2 || true
    fail "installed Android app did not complete native realtime lifecycle proof"
  fi

  launch_realtime
  for _ in $(seq 1 60); do
    if realtime_ready "$rejoin_output"; then
      echo "Android installed-app native realtime lifecycle + restart/rejoin: PASS"
      return 0
    fi
    sleep 1
  done

  [[ -f "$rejoin_output" ]] && cat "$rejoin_output" >&2 || true
  fail "installed Android app did not rejoin the same canonical consultation after process restart"
}

assert_installation_lifecycle
assert_deep_link_runtime
assert_notification_runtime
assert_offline_mutation_restart
assert_realtime_lifecycle

echo "ANDROID CAPABILITY + INSTALLATION + DEEP-LINK + NOTIFICATION + OFFLINE-SYNC + REALTIME NATIVE E2E: PASS"