import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r9_sql_contract_guard import validate_sql_text


class R9SQLGuardTest(unittest.TestCase):
    def test_well_formed_named_blocks_and_multiple_statements(self):
        sql = (
            "CREATE FUNCTION f() RETURNS void AS $body$ BEGIN NULL; END; $body$ LANGUAGE plpgsql;\n"
            "DO $r9$ BEGIN NULL; END; $r9$;\n"
            "SELECT $data$opaque$\n$data$;"
        )
        self.assertEqual(validate_sql_text(sql), [])

    def test_regression_unbalanced_do_dollar(self):
        broken = "DO $\nBEGIN\n NULL;\nEND;\n$$;"
        self.assertTrue(validate_sql_text(broken))

    def test_regression_single_dollar_close(self):
        broken = "DO $$ BEGIN NULL; END; $;"
        self.assertTrue(validate_sql_text(broken))

    def test_mismatched_named_delimiters(self):
        broken = "DO $r9_a$ BEGIN NULL; END; $r9_b$;"
        self.assertTrue(validate_sql_text(broken))

    def test_balanced_anonymous_blocks(self):
        self.assertEqual(validate_sql_text("DO $$ BEGIN NULL; END; $$;"), [])


if __name__ == "__main__":
    unittest.main()
