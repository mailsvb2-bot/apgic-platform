from __future__ import annotations

import unittest

from tools.auth_contract_guard import validate_authorization_contract


class AuthorizationContractGuardTests(unittest.TestCase):
    def test_accepts_exact_decision_and_risk_parity(self) -> None:
        go = '''
const (
    Allow Decision = "ALLOW"
    Deny Decision = "DENY"
    StepUpRequired Decision = "STEP_UP_REQUIRED"
    RiskNormal Risk = "NORMAL"
    RiskHigh Risk = "HIGH_RISK"
)
'''
        ts = 'export type AuthorizationDecision = "ALLOW" | "DENY" | "STEP_UP_REQUIRED";'
        schema = {
            "$defs": {
                "AuthorizationDecision": {"enum": ["ALLOW", "DENY", "STEP_UP_REQUIRED"]},
                "Risk": {"enum": ["NORMAL", "HIGH_RISK"]},
            }
        }
        self.assertEqual(validate_authorization_contract(go, ts, schema), [])

    def test_rejects_client_decision_drift(self) -> None:
        go = '''
const (
    Allow Decision = "ALLOW"
    Deny Decision = "DENY"
    StepUpRequired Decision = "STEP_UP_REQUIRED"
    RiskNormal Risk = "NORMAL"
    RiskHigh Risk = "HIGH_RISK"
)
'''
        ts = 'export type AuthorizationDecision = "ALLOW" | "DENY" | "REQUIRE_STEP_UP";'
        schema = {
            "$defs": {
                "AuthorizationDecision": {"enum": ["ALLOW", "DENY", "STEP_UP_REQUIRED"]},
                "Risk": {"enum": ["NORMAL", "HIGH_RISK"]},
            }
        }
        errors = validate_authorization_contract(go, ts, schema)
        self.assertEqual(len(errors), 1)
        self.assertIn("Go/TypeScript authorization decisions differ", errors[0])

    def test_rejects_schema_risk_drift(self) -> None:
        go = '''
const (
    Allow Decision = "ALLOW"
    Deny Decision = "DENY"
    StepUpRequired Decision = "STEP_UP_REQUIRED"
    RiskNormal Risk = "NORMAL"
    RiskHigh Risk = "HIGH_RISK"
)
'''
        ts = 'export type AuthorizationDecision = "ALLOW" | "DENY" | "STEP_UP_REQUIRED";'
        schema = {
            "$defs": {
                "AuthorizationDecision": {"enum": ["ALLOW", "DENY", "STEP_UP_REQUIRED"]},
                "Risk": {"enum": ["NORMAL"]},
            }
        }
        errors = validate_authorization_contract(go, ts, schema)
        self.assertEqual(len(errors), 1)
        self.assertIn("Go/JSON Schema authorization risks differ", errors[0])


if __name__ == "__main__":
    unittest.main()
