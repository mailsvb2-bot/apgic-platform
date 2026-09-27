import copy
import json
import unittest

from tools.ledger_contract_guard import (
    CI_INVARIANTS,
    GO_LEDGER,
    IDEMPOTENCY_MIGRATION,
    MIGRATION,
    SCHEMA,
    validate_ledger_contract,
)


class LedgerContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_LEDGER.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.ci_invariants = CI_INVARIANTS.read_text(encoding="utf-8")
        self.idempotency_migration = IDEMPOTENCY_MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def validate(self, go=None, migration=None, schema=None, ci_invariants=None, idempotency_migration=None):
        return validate_ledger_contract(
            self.go if go is None else go,
            self.migration if migration is None else migration,
            self.schema if schema is None else schema,
            self.ci_invariants if ci_invariants is None else ci_invariants,
            self.idempotency_migration if idempotency_migration is None else idempotency_migration,
        )

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(self.validate(), [])

    def test_provider_evidence_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("provider_evidence_ref")
        errors = self.validate(schema=broken)
        self.assertTrue(any("required fields differ" in error for error in errors))

    def test_currency_contract_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["currency"]["pattern"] = ".*"
        errors = self.validate(schema=broken)
        self.assertTrue(any("currency" in error for error in errors))

    def test_missing_append_only_trigger_is_rejected(self):
        broken_sql = self.migration.replace(
            "CREATE TRIGGER ledger_entries_append_only",
            "CREATE TRIGGER ledger_entries_mutable",
        )
        errors = self.validate(migration=broken_sql)
        self.assertTrue(any("SQL invariant missing" in error for error in errors))

    def test_missing_replay_logic_is_rejected(self):
        broken_go = self.go.replace("func Reconcile(entries []Entry)", "func Replay(entries []Entry)")
        errors = self.validate(go=broken_go)
        self.assertTrue(any("replay invariant missing" in error for error in errors))

    def test_null_unsafe_db_balance_assertion_is_rejected(self):
        broken_invariants = self.ci_invariants.replace(
            "IS DISTINCT FROM",
            "<>",
        )
        errors = self.validate(ci_invariants=broken_invariants)
        self.assertTrue(any("DB reconciliation proof missing" in error for error in errors))

    def test_missing_db_reconciliation_proof_is_rejected(self):
        broken_invariants = self.ci_invariants.replace(
            "ledger replay balances do not reconcile exactly in minor units",
            "ledger replay balance assertion removed",
        )
        errors = self.validate(ci_invariants=broken_invariants)
        self.assertTrue(any("DB reconciliation proof missing" in error for error in errors))

    def test_missing_economic_event_unique_index_is_rejected(self):
        broken = self.idempotency_migration.replace(
            "ledger_entries_economic_event_ref_unique",
            "ledger_entries_economic_event_ref_not_unique",
        )
        errors = self.validate(idempotency_migration=broken)
        self.assertTrue(any("idempotency invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
