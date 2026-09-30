#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APK="$ROOT/apps/mobile/android/app/build/outputs/apk/debug/app-debug.apk"
EVIDENCE_DIR="$ROOT/evidence"
SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
AVD_NAME="apgic-native-e2e"
SYSTEM_IMAGE="${APGIC_ANDROID_E2E_IMAGE:-system-images;android-36;google_apis;x86_64}"
METRO_LOG="/tmp/apgic-metro-android.log"
EMULATOR_LOG="/tmp/apgic-emulator.log"
METRO_PID=""
EMULATOR_PID=""

fail() {
  echo "ANDROID CAPABILITY NATIVE E2E: FAIL: $*" >&2
  if [[ -f "$METRO_LOG" ]]; then
    tail -n 120 "$METRO_LOG" >&2 || true
  fi
  if [[ -f "$EMULATOR_LOG" ]]; then
    tail -n 120 "$EMULATOR_LOG" >&2 || true
  fi
  exit 1
}

cleanup() {
  if [[ -n "$METRO_PID" ]]; then
    kill "$METRO_PID" 2>/dev/null || true
  fi
  adb emu kill >/dev/null 2>&1 || true
  if [[ -n "$EMULATOR_PID" ]]; then
    kill "$EMULATOR_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

[[ -f "$APK" ]] || fail "debug APK missing: $APK"
[[ -n "$SDK_ROOT" ]] || fail "ANDROID_SDK_ROOT/ANDROID_HOME is not set"

SDKMANAGER="$SDK_ROOT/cmdline-tools/latest/bin/sdkmanager"
AVDMANAGER="$SDK_ROOT/cmdline-tools/latest/bin/avdmanager"
EMULATOR="$SDK_ROOT/emulator/emulator"
[[ -x "$SDKMANAGER" ]] || SDKMANAGER="$(command -v sdkmanager || true)"
[[ -x "$AVDMANAGER" ]] || AVDMANAGER="$(command -v avdmanager || true)"
[[ -x "$EMULATOR" ]] || EMULATOR="$(command -v emulator || true)"
[[ -x "$SDKMANAGER" ]] || fail "sdkmanager not found"
[[ -x "$AVDMANAGER" ]] || fail "avdmanager not found"
[[ -x "$EMULATOR" ]] || fail "emulator not found"

yes | "$SDKMANAGER" --licenses >/dev/null || true
"$SDKMANAGER" "platform-tools" "emulator" "$SYSTEM_IMAGE"
"$AVDMANAGER" delete avd -n "$AVD_NAME" >/dev/null 2>&1 || true
echo "no" | "$AVDMANAGER" create avd --force -n "$AVD_NAME" -k "$SYSTEM_IMAGE"

"$EMULATOR"   -avd "$AVD_NAME"   -no-window   -no-audio   -no-snapshot   -no-boot-anim   -gpu swiftshader_indirect   >"$EMULATOR_LOG" 2>&1 &
EMULATOR_PID=$!

adb wait-for-device
for _ in $(seq 1 120); do
  if [[ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]]; then
    break
  fi
  sleep 2
done
[[ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]] ||
  fail "emulator did not finish booting"

adb shell settings put global window_animation_scale 0
adb shell settings put global transition_animation_scale 0
adb shell settings put global animator_duration_scale 0

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

adb reverse tcp:8081 tcp:8081
adb install -r "$APK" >/dev/null
mkdir -p "$EVIDENCE_DIR"

assert_state() {
  local state="$1"
  local reason="$2"
  local remote="/sdcard/apgic-capability-${state}.xml"
  local local_file="$EVIDENCE_DIR/android-capability-e2e-${state}.xml"

  adb shell am force-stop com.apgic.ci
  adb shell am start -W     -n com.apgic.ci/.MainActivity     --es APGIC_E2E_CAPABILITY_STATE "$state"     >/dev/null

  for _ in $(seq 1 30); do
    if adb shell uiautomator dump "$remote" >/dev/null 2>&1 &&
       adb pull "$remote" "$local_file" >/dev/null 2>&1 &&
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

echo "ANDROID CAPABILITY NATIVE E2E: PASS"
