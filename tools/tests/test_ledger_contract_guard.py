import copy
import json
import unittest

from tools.ledger_contract_guard import (
    CI_INVARIANTS,
    GO_LEDGER,
    MIGRATION,
    SCHEMA,
    validate_ledger_contract,
)


class LedgerContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_LEDGER.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.ci_invariants = CI_INVARIANTS.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(
            validate_ledger_contract(
                self.go,
                self.migration,
                self.schema,
                self.ci_invariants,
            ),
            [],
        )

    def test_provider_evidence_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("provider_evidence_ref")
        errors = validate_ledger_contract(
            self.go,
            self.migration,
            broken,
            self.ci_invariants,
        )
        self.assertTrue(any("required fields differ" in error for error in errors))

    def test_currency_contract_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["currency"]["pattern"] = ".*"
        errors = validate_ledger_contract(
            self.go,
            self.migration,
            broken,
            self.ci_invariants,
        )
        self.assertTrue(any("currency" in error for error in errors))

    def test_missing_append_only_trigger_is_rejected(self):
        broken_sql = self.migration.replace(
            "CREATE TRIGGER ledger_entries_append_only",
            "CREATE TRIGGER ledger_entries_mutable",
        )
        errors = validate_ledger_contract(
            self.go,
            broken_sql,
            self.schema,
            self.ci_invariants,
        )
        self.assertTrue(any("SQL invariant missing" in error for error in errors))

    def test_missing_replay_logic_is_rejected(self):
        broken_go = self.go.replace("func Reconcile(entries []Entry)", "func Replay(entries []Entry)")
        errors = validate_ledger_contract(
            broken_go,
            self.migration,
            self.schema,
            self.ci_invariants,
        )
        self.assertTrue(any("replay invariant missing" in error for error in errors))

    def test_null_unsafe_db_balance_assertion_is_rejected(self):
        broken_invariants = self.ci_invariants.replace(
            "IS DISTINCT FROM",
            "<>",
        )
        errors = validate_ledger_contract(
            self.go,
            self.migration,
            self.schema,
            broken_invariants,
        )
        self.assertTrue(any("DB reconciliation proof missing" in error for error in errors))

    def test_missing_db_reconciliation_proof_is_rejected(self):
        broken_invariants = self.ci_invariants.replace(
            "ledger replay balances do not reconcile exactly in minor units",
            "ledger replay balance assertion removed",
        )
        errors = validate_ledger_contract(
            self.go,
            self.migration,
            self.schema,
            broken_invariants,
        )
        self.assertTrue(any("DB reconciliation proof missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
