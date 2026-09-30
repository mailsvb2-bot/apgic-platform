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
ADB=""

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
  if [[ -n "$ADB" ]]; then
    "$ADB" emu kill >/dev/null 2>&1 || true
  fi
  if [[ -n "$EMULATOR_PID" ]]; then
    kill "$EMULATOR_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

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

"$ADB" reverse tcp:8081 tcp:8081
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

echo "ANDROID CAPABILITY NATIVE E2E: PASS"
