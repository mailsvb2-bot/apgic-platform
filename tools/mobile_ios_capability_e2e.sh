#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="$ROOT/apps/mobile/ios/build/derived/Build/Products/Debug-iphonesimulator/APGIC.app"
EVIDENCE_DIR="$ROOT/evidence"
METRO_LOG="/tmp/apgic-metro-ios.log"
METRO_PID=""
UDID=""

fail() {
  echo "IOS CAPABILITY NATIVE E2E: FAIL: $*" >&2
  if [[ -f "$METRO_LOG" ]]; then
    tail -n 120 "$METRO_LOG" >&2 || true
  fi
  exit 1
}

cleanup() {
  if [[ -n "$METRO_PID" ]]; then
    kill "$METRO_PID" 2>/dev/null || true
  fi
  if [[ -n "$UDID" ]]; then
    xcrun simctl shutdown "$UDID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

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

if ! command -v idb >/dev/null 2>&1; then
  brew install facebook/fb/idb
fi
IDB="$(command -v idb)"
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

echo "IOS CAPABILITY NATIVE E2E: PASS"
