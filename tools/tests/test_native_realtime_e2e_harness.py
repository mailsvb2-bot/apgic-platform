from __future__ import annotations

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
E2E = ROOT / "apps/mobile/src/r3-realtime-e2e.ts"
APP = ROOT / "apps/mobile/src/App.tsx"
ANDROID = ROOT / "tools/mobile_android_capability_e2e.sh"
IOS = ROOT / "tools/mobile_ios_capability_e2e.sh"


class NativeRealtimeE2EHarnessTests(unittest.TestCase):
    def test_transport_handoff_is_proven_separately_from_ambient_final_transport(self) -> None:
        e2e = E2E.read_text(encoding="utf-8")
        app = APP.read_text(encoding="utf-8")
        android = ANDROID.read_text(encoding="utf-8")
        ios = IOS.read_text(encoding="utf-8")

        self.assertIn("networkTransportHistory", e2e)
        self.assertIn('result.reason_code === "REALTIME_NETWORK_TRANSPORT_CHANGED"', e2e)
        self.assertIn("networkTransportHistory.push(result.snapshot.network_transport)", e2e)
        self.assertIn("realtime-network-transport-observed:", app)

        self.assertIn("NETWORK_TRANSPORT_CHANGED:CELLULAR", android)
        self.assertIn("realtime-network-transport-observed:", android)
        self.assertIn("CELLULAR", android)
        self.assertIn(
            "realtime-network-transport:(WIFI|CELLULAR|ETHERNET|OTHER|UNKNOWN)",
            android,
        )
        self.assertNotIn(
            "grep -q 'realtime-network-transport:CELLULAR'",
            android,
        )

        self.assertIn("NETWORK_TRANSPORT_CHANGED:CELLULAR", ios)
        self.assertIn(
            'json_has_ax_label_fragment "$target" "realtime-network-transport-observed:" "CELLULAR"',
            ios,
        )
        self.assertNotIn(
            'json_has_ax_label "$target" "realtime-network-transport:CELLULAR"',
            ios,
        )


if __name__ == "__main__":
    unittest.main()
