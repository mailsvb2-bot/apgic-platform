"""Regression guards for the 47 retired APGIC source heads and their safe replacements."""
import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "archived_functional_reconciliation.py"
spec = importlib.util.spec_from_file_location("archived_functional_reconciliation", SCRIPT)
assert spec and spec.loader
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class ArchivedFunctionalReconciliationTests(unittest.TestCase):
    def test_all_renumbered_migrations_have_unique_canonical_targets(self):
        self.assertEqual(len(audit.RENAMED_SQL), 9)
        self.assertEqual(len(set(audit.RENAMED_SQL.values())), 9)
        self.assertTrue(all(a != b for a, b in audit.RENAMED_SQL.items()))

    def test_semantically_identical_renumbered_sql_can_be_reused(self):
        path = "backend/migrations/000044_r9_live_events.sql"
        with patch.object(audit, "blob", return_value="SELECT 1;\n"):
            result = audit.classify_absent(path, "SELECT 1;\n", "a"*40, repo=Path("."))
        self.assertEqual(result["disposition"], "RENUMBERED_SAME_SQL")

    def test_non_equivalent_migration_is_not_silently_accepted(self):
        path = "backend/migrations/000044_r9_live_events.sql"
        with patch.object(audit, "blob", return_value="DROP TABLE live_events;"):
            with self.assertRaisesRegex(ValueError, "changed without approved"):
                audit.classify_absent(path, "CREATE TABLE live_events(id uuid);", "a"*40, repo=Path("."))

    def test_signed_payment_replacement_must_preserve_trigger_and_receipts(self):
        path = "backend/migrations/000049_r2_r3_signed_payment_admission.sql"
        with patch.object(audit, "blob", return_value="apgic_live_consultation_payment_authorized"):
            with self.assertRaisesRegex(ValueError, "changed without approved"):
                audit.classify_absent(path, "OLD", "a"*40, repo=Path("."))
        strong = ("apgic_live_consultation_payment_authorized\n"
                  "apgic_live_join_signed_capture_insert_guard\n"
                  "payment_webhook_receipts")
        with patch.object(audit, "blob", return_value=strong):
            result = audit.classify_absent(path, "OLDER SQL", "a"*40, repo=Path("."))
        self.assertEqual(result["disposition"], "RENUMBERED_STRONGER_SIGNED_PAYMENT_GUARD")

    def test_unexpected_missing_runtime_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "Unclassified absent"):
            audit.classify_absent("backend/cmd/new-runtime/main.go", "func main(){}", "a"*40, repo=Path("."))

    def test_old_deletion_worker_maps_to_only_one_active_runtime(self):
        path = "backend/cmd/deletion-worker/main.go"
        with patch.object(audit, "blob", return_value=None):
            with self.assertRaisesRegex(ValueError, "Missing replacement"):
                audit.classify_absent(path, "func main(){}", "a"*40, repo=Path("."))
        with patch.object(audit, "blob", return_value="type DeletionRuntime struct {}"):
            result = audit.classify_absent(path, "func main(){}", "a"*40, repo=Path("."))
        self.assertEqual(result["disposition"], "REPLACED_BY_SINGLE_CANONICAL_R5")

    def test_auth001_replacement_requires_real_postgres_browser_guard(self):
        with patch.object(audit, "blob", return_value="unit tests only"):
            with self.assertRaisesRegex(ValueError, "Missing live browser"):
                audit.classify_absent(audit.LEGACY_AUTH, "script", "a"*40, repo=Path("."))

    def test_old_docs_are_not_mistaken_for_missing_executors(self):
        for path in audit.LEGACY_DOCS:
            result = audit.classify_absent(path, "historic document", "a"*40, repo=Path("."))
            self.assertEqual(result["disposition"], "RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME")


if __name__ == "__main__":
    unittest.main()
