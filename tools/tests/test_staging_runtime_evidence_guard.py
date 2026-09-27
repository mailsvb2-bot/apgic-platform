import copy
import json
import unittest

from tools.staging_runtime_evidence_guard import (
    EVIDENCE_DIR,
    PINNED_ARTIFACTS,
    SCHEMA,
    _is_rfc3339_datetime,
    _pin_errors,
    _registry_evidence_refs,
    _schema_errors,
    discover_evidence_paths,
    validate,
)


class StagingRuntimeEvidenceGuardTest(unittest.TestCase):
    def test_all_staging_runtime_evidence_is_scoped_and_bound(self):
        self.assertEqual(validate(), [])

    def test_guard_discovers_exactly_reviewed_staging_evidence(self):
        names = {path.name for path in discover_evidence_paths()}
        self.assertEqual(names, set(PINNED_ARTIFACTS))

    def test_nested_schema_validation_rejects_missing_tls_expiry_and_bad_date(self):
        schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        path = EVIDENCE_DIR / "staging-runtime-20260927T072352Z.json"
        document = json.loads(path.read_text(encoding="utf-8"))
        broken = copy.deepcopy(document)
        broken["tls"].pop("not_after")
        broken["generated_at"] = "not-a-date"
        broken["runtime"]["undeclared"] = True

        errors = _schema_errors(broken, schema, path.name)
        joined = "\n".join(errors)
        self.assertIn("not_after", joined)
        self.assertIn("generated_at", joined)
        self.assertIn("undeclared", joined)

    def test_rfc3339_fallback_rejects_noncanonical_iso_forms(self):
        self.assertTrue(_is_rfc3339_datetime("2026-09-27T07:23:52Z"))
        self.assertTrue(_is_rfc3339_datetime("2026-09-27t07:23:52z"))
        self.assertTrue(_is_rfc3339_datetime("2016-12-31T23:59:60Z"))
        self.assertFalse(_is_rfc3339_datetime("2016-12-31T23:59:61Z"))
        self.assertTrue(_is_rfc3339_datetime("2026-09-27T07:23:52.123+03:00"))
        self.assertFalse(_is_rfc3339_datetime("20260927T072352Z"))
        self.assertFalse(_is_rfc3339_datetime("2026-09-27T07:23:52+03"))
        self.assertFalse(_is_rfc3339_datetime("2026-09-27T07:23:52+03:00:30"))
        self.assertFalse(_is_rfc3339_datetime("2026-02-30T07:23:52Z"))

    def test_registry_linkage_uses_only_actual_evidence_refs(self):
        ref = "canon/evidence/staging-runtime-example.json"
        parsed = _registry_evidence_refs({
            "requirements": [
                {
                    "requirement_id": "APGIC-EXEC-001",
                    "statement": f"comment mentions {ref}",
                    "implementation_refs": [ref],
                    "evidence_refs": [],
                },
                {
                    "requirement_id": "APGIC-NFR-001",
                    "evidence_refs": [ref],
                },
            ]
        })
        self.assertNotIn(ref, parsed["APGIC-EXEC-001"])
        self.assertIn(ref, parsed["APGIC-NFR-001"])

    def test_coordinated_identity_rewrite_breaks_immutable_digest(self):
        filename = "staging-runtime-20260926T215500Z.json"
        path = EVIDENCE_DIR / filename
        document = json.loads(path.read_text(encoding="utf-8"))
        broken = copy.deepcopy(document)
        broken["candidate_sha"] = "a" * 40
        broken["runtime"]["meta_commit_sha"] = "a" * 40
        broken["live_e2e"]["tested_sha"] = "a" * 40
        broken["deployment_identity"] = "staging-aaaaaaaa-20990101T0000Z"
        raw = (json.dumps(broken, sort_keys=True) + "\n").encode("utf-8")

        errors = _pin_errors(filename, raw, broken)
        joined = "\n".join(errors)
        self.assertIn("immutable digest mismatch", joined)
        self.assertIn("candidate SHA changed", joined)
        self.assertIn("deployment identity changed", joined)


if __name__ == "__main__":
    unittest.main()
