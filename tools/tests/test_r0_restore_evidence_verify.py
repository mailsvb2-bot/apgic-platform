from __future__ import annotations

import importlib.util
import json
import subprocess
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "r0_restore_evidence_verify.py"
SPEC = importlib.util.spec_from_file_location("r0_restore_evidence_verify", SCRIPT)
assert SPEC and SPEC.loader
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)
SHA = "a" * 40


class RestoreEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.now = datetime(2026, 10, 18, tzinfo=timezone.utc)
        self.doc = {
            "schema_version": "staging-restore-evidence-v2",
            "evidence_type": "STAGING_RESTORE_DRILL",
            "candidate_sha": SHA,
            "observed_at": (self.now - timedelta(minutes=5)).isoformat().replace("+00:00", "Z"),
            "backup_file_sha256": "b" * 64,
            "measured_backup_rpo_seconds": 60,
            "measured_restore_rto_ms": 1100,
            "source_table_count": 23,
            "restored_table_count": 23,
            "required_table_count": 7,
            "business_probe_identity_count": 2,
            "business_probe_organization_count": 1,
            "business_probes_passed": True,
            "integrity_probes_passed": True,
            "production_evidence": False,
        }
        for probe in gate.INTEGRITY_PROBES:
            self.doc["integrity_probe_" + probe + "_trigger_count"] = 1

    def assert_blocked(self, field, value):
        doc = dict(self.doc)
        doc[field] = value
        self.assertTrue(gate.verify(doc, SHA, 86400, self.now), field)

    def test_real_format_acceptance_only(self):
        self.assertEqual(gate.verify(self.doc, SHA, 86400, self.now), [])

    def test_rejects_unbound_unmeasured_or_false_positive_evidence(self):
        tests = {
            "candidate_sha": "b" * 40,
            "observed_at": "2020-01-01T00:00:00Z",
            "backup_file_sha256": "not a digest",
            "measured_backup_rpo_seconds": -1,
            "measured_restore_rto_ms": "not measured",
            "source_table_count": 0,
            "restored_table_count": 24,
            "required_table_count": False,
            "business_probe_identity_count": 0,
            "business_probe_organization_count": 0,
            "integrity_probe_audit_trigger_count": 0,
            "integrity_probe_ledger_trigger_count": 2,
            "business_probes_passed": False,
            "integrity_probes_passed": False,
            "production_evidence": True,
            "schema_version": "legacy-or-fake",
        }
        for field, value in tests.items():
            with self.subTest(field=field):
                self.assert_blocked(field, value)

    def test_future_and_malformed_timestamps_fail(self):
        for value in ("invalid", "2026-10-18T00:00:00", "2035-01-01T00:00:00Z"):
            with self.subTest(value=value):
                self.assert_blocked("observed_at", value)

    def test_actual_staging_shell_uses_proven_validator(self):
        path = SCRIPT.parent.parent / "deploy/staging/prove-r0-foundation-readonly.sh"
        subprocess.run(["bash", "-n", str(path)], check=True)
        source = path.read_text(encoding="utf-8")
        self.assertIn("r0_restore_evidence_verify.py", source)
        self.assertIn("assert-authorized-host.sh", source)
        self.assertIn("check-staging-runtime.sh", source)
        self.assertNotIn("find \"$RESTORE_DIR\"", source)

if __name__ == "__main__":
    unittest.main()
