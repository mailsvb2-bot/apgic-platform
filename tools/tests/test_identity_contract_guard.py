from __future__ import annotations

import unittest

from tools.identity_contract_guard import validate_identity_role_parity


class IdentityContractGuardTests(unittest.TestCase):
    def test_accepts_exact_cross_language_role_parity(self) -> None:
        go = 'const (\nRoleClient Role = "CLIENT"\nRoleStudent Role = "STUDENT"\n)'
        ts = 'export type IdentityRole = "CLIENT" | "STUDENT";'
        schema = {"properties": {"roles": {"items": {"enum": ["CLIENT", "STUDENT"]}}}}
        self.assertEqual(validate_identity_role_parity(go, ts, schema), [])

    def test_rejects_client_only_role(self) -> None:
        go = 'const (\nRoleClient Role = "CLIENT"\n)'
        ts = 'export type IdentityRole = "CLIENT" | "ORGANIZATION_MEMBER";'
        schema = {"properties": {"roles": {"items": {"enum": ["CLIENT"]}}}}
        errors = validate_identity_role_parity(go, ts, schema)
        self.assertEqual(len(errors), 1)
        self.assertIn("Go/TypeScript identity roles differ", errors[0])

    def test_rejects_schema_role_drift(self) -> None:
        go = 'const (\nRoleClient Role = "CLIENT"\n)'
        ts = 'export type IdentityRole = "CLIENT";'
        schema = {"properties": {"roles": {"items": {"enum": ["CLIENT", "AUTHOR"]}}}}
        errors = validate_identity_role_parity(go, ts, schema)
        self.assertEqual(len(errors), 1)
        self.assertIn("Go/JSON Schema identity roles differ", errors[0])


if __name__ == "__main__":
    unittest.main()
