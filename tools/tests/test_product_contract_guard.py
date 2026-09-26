import copy
import json
import unittest

from tools.product_contract_guard import (
    GO_PRODUCT,
    MIGRATION_FOUNDATION,
    MIGRATION_SEMANTICS,
    SCHEMA,
    validate_product_contract,
)


class ProductContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_PRODUCT.read_text(encoding="utf-8")
        self.foundation = MIGRATION_FOUNDATION.read_text(encoding="utf-8")
        self.semantics = MIGRATION_SEMANTICS.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(
            validate_product_contract(self.go, self.foundation, self.semantics, self.schema),
            [],
        )

    def test_owner_type_drift_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["owner_type"]["enum"].remove("ORGANIZATION")
        errors = validate_product_contract(self.go, self.foundation, self.semantics, broken)
        self.assertTrue(any("owner types differ" in error for error in errors))

    def test_author_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["author_refs"]["minItems"] = 0
        errors = validate_product_contract(self.go, self.foundation, self.semantics, broken)
        self.assertTrue(any("author_refs" in error for error in errors))

    def test_owner_existence_trigger_is_required(self):
        broken_sql = self.semantics.replace("CREATE TRIGGER products_owner_exists", "CREATE TRIGGER products_owner_unchecked")
        errors = validate_product_contract(self.go, self.foundation, broken_sql, self.schema)
        self.assertTrue(any("semantic SQL invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
