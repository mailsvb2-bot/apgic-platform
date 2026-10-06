from __future__ import annotations

import unittest

from tools.mobile_realtime_guard import validate_callback_order_evidence


class MobileRealtimeEvidenceGuardTests(unittest.TestCase):
    def good(self) -> tuple[str, str, str, str]:
        realtime = """
audioRoutesObserved
networkTransportsObserved
audioRoutesObserved.add(result.snapshot.audio_route)
networkTransportsObserved.add(result.snapshot.network_transport)
"""
        app = """
realtime-audio-routes-observed:
realtime-network-transports-observed:
"""
        android = """
grep -Eq 'realtime-audio-route:(SPEAKER|EARPIECE|BLUETOOTH|WIRED|UNKNOWN)'
grep -Eq 'realtime-audio-routes-observed:[^"]*BLUETOOTH'
grep -Eq 'realtime-network-transport:(WIFI|CELLULAR|ETHERNET|OTHER|UNKNOWN)'
grep -Eq 'realtime-network-transports-observed:[^"]*CELLULAR'
"""
        ios = """
json_has_ax_label_contains "$target" "realtime-audio-route:" ""
json_has_ax_label_contains "$target" "realtime-audio-routes-observed:" "BLUETOOTH"
json_has_ax_label_contains "$target" "realtime-network-transport:" ""
json_has_ax_label_contains "$target" "realtime-network-transports-observed:" "CELLULAR"
"""
        return realtime, app, android, ios

    def test_callback_order_safe_evidence_passes(self) -> None:
        validate_callback_order_evidence(*self.good())

    def test_android_synthetic_final_transport_is_rejected(self) -> None:
        realtime, app, android, ios = self.good()
        android += "\ngrep -q 'realtime-network-transport:CELLULAR' \"$target\"\n"
        with self.assertRaises(SystemExit):
            validate_callback_order_evidence(realtime, app, android, ios)

    def test_ios_synthetic_final_audio_route_is_rejected(self) -> None:
        realtime, app, android, ios = self.good()
        ios += '\njson_has_ax_label "$target" "realtime-audio-route:BLUETOOTH"\n'
        with self.assertRaises(SystemExit):
            validate_callback_order_evidence(realtime, app, android, ios)


if __name__ == "__main__":
    unittest.main()
