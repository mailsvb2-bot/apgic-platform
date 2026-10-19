from __future__ import annotations

import importlib.util
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tools/r0_r3_live_readback.py"
spec = importlib.util.spec_from_file_location("r0_r3_live_readback", SCRIPT)
assert spec and spec.loader
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)
SHA = "a"*40
ID = "00000000-0000-4000-8000-000000000901"


class R0R3LiveReadbackTests(unittest.TestCase):
    def valid(self):
        return ({"commit_sha":SHA,"service":"apgic-api"},
                {"profiles":[{"id":ID,"display_name":"Real Specialist"}]},
                {"booking_id":ID,"state":"CONFIRMED"},
                {"booking_id":ID,"state":"COMPLETED","completion_evidence_ref":"provider-signed/completed"})

    def test_exact_candidate_readback_contract(self):
        self.assertEqual(probe.verify_observations(*self.valid(), SHA, ID), [])

    def test_absent_real_data_never_passes(self):
        cases=[
            (0, "commit_sha", "b"*40, "R0_RUNTIME_SHA_MISMATCH"),
            (1, "profiles", [], "R1_NO_REAL_PUBLISHED_SPECIALISTS"),
            (2, "booking_id", "bad", "R2_BOOKING_ID_MISMATCH"),
            (2, "state", "HELD", "R2_BOOKING_NOT_CONFIRMED"),
            (3, "booking_id", "bad", "R3_CONSULTATION_ID_MISMATCH"),
            (3, "state", "JOINED", "R3_CONSULTATION_NOT_COMPLETED"),
            (3, "completion_evidence_ref", "", "R3_PROVIDER_COMPLETION_EVIDENCE_MISSING"),
        ]
        for index,key,value,expected in cases:
            with self.subTest(expected=expected):
                data=list(self.valid())
                data[index][key]=value
                self.assertIn(expected,probe.verify_observations(*data,SHA,ID))

    def test_authenticated_probe_never_accepts_nonlocal_api_origin(self):
        from unittest import mock
        safe = ("http://127.0.0.1:43111", "http://[::1]:43111")
        for origin in safe:
            with self.subTest(origin=origin):
                probe.assert_local_api_base(origin)
        rejected = (
            "https://127.0.0.1:43111", "http://example.com:43111",
            "http://192.0.2.1:43111", "http://localhost:43111",
            "http://127.0.0.1", "http://127.0.0.1:43111/path",
            "http://" + "user" + ":" + "password" + "@127.0.0.1:43111",
            "http://127.0.0.1:43111/?redirect=1",
        )
        for origin in rejected:
            with self.subTest(origin=origin):
                with self.assertRaises(probe.ProbeError):
                    probe.assert_local_api_base(origin)
        with self.assertRaises(probe.ProbeError):
            probe.NoRedirect().redirect_request(None, None, 302, "Moved", {}, "https://evil.example")
        with mock.patch.object(probe.urllib.request, "build_opener") as open_mock:
            with self.assertRaises(probe.ProbeError):
                probe.fetch_json("http://example.org:43111", "/v1/bookings/1", "session=secret")
            open_mock.assert_not_called()

    def test_authorized_host_guard_and_script_syntax(self):
        path = ROOT / "deploy/staging/prove-r0-r3-client-readback.sh"
        subprocess.run(["bash","-n",str(path)],check=True)
        source=path.read_text(encoding="utf-8")
        for required in ("assert-authorized-host.sh","check-staging-runtime.sh",
                         "APGIC_R0_R3_ACCEPTANCE_SESSION_COOKIE",
                         "APGIC_R0_R3_ACCEPTANCE_BOOKING_ID", "r0_r3_live_readback.py"):
            self.assertIn(required,source)

if __name__ == "__main__":
    unittest.main()
