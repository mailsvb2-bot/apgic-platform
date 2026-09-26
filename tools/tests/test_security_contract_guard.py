import copy
import json
import unittest

from tools.security_contract_guard import (
    ARCH_GUARD,
    GO_SECURITY,
    SCHEMA,
    validate_security_contract,
)


class SecurityContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_SECURITY.read_text(encoding="utf-8")
        self.arch = ARCH_GUARD.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_contract_matches_go_and_architecture_guard(self):
        self.assertEqual(validate_security_contract(self.go, self.arch, self.schema), [])

    def test_wildcard_scope_is_forbidden(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["scopes"]["items"].pop("not")
        errors = validate_security_contract(self.go, self.arch, broken)
        self.assertTrue(any("wildcard scope" in error for error in errors))

    def test_empty_scope_set_is_forbidden(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["scopes"]["minItems"] = 0
        errors = validate_security_contract(self.go, self.arch, broken)
        self.assertTrue(any("at least one scope" in error for error in errors))

    def test_secret_env_scan_is_required(self):
        broken_arch = self.arch.replace('for path in ROOT.rglob(".env*")', 'for path in []')
        errors = validate_security_contract(self.go, broken_arch, self.schema)
        self.assertTrue(any("secret-bearing env files" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
