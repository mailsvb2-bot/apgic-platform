from __future__ import annotations

import unittest

from tools.auth_contract_guard import (
    validate_authorization_contract,
    validate_client_session_contract,
)


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


    def test_client_session_contract_accepts_trusted_cookie_and_deprecated_ids(self) -> None:
        document = {
            "components": {
                "securitySchemes": {
                    "ClientSession": {
                        "type": "apiKey",
                        "in": "cookie",
                        "name": "__Host-apgic_session",
                    }
                },
                "schemas": {
                    "AcquireSlotHoldRequest": {
                        "required": ["help_intent_id", "slot_id"],
                        "properties": {"client_identity_id": {"deprecated": True}},
                    },
                    "CreateCheckoutInstructionRequest": {
                        "required": ["hold_id", "method_code"],
                        "properties": {"client_identity_id": {"deprecated": True}},
                    },
                    "AccountDeletionRequest": {
                        "required": ["source"],
                        "properties": {"identity_id": {"deprecated": True}},
                    },
                },
            },
            "paths": {
                "/v1/slot-holds": {"post": {"security": [{"ClientSession": []}]}},
                "/v1/slot-holds/{id}/checkout-options": {
                    "get": {
                        "security": [{"ClientSession": []}],
                        "parameters": [
                            {
                                "name": "client_identity_id",
                                "in": "query",
                                "required": False,
                                "deprecated": True,
                            }
                        ],
                    }
                },
                "/v1/checkout-instructions": {"post": {"security": [{"ClientSession": []}]}},
                "/v1/account-deletions": {"post": {"security": [{"ClientSession": []}]}},
                "/v1/bookings/{id}/fulfillment": {
                    "get": {
                        "security": [{"ClientSession": []}],
                        "parameters": [
                            {
                                "name": "identity_id",
                                "in": "query",
                                "required": False,
                                "deprecated": True,
                            }
                        ],
                    }
                },
            },
        }
        self.assertEqual(validate_client_session_contract(document), [])

    def test_client_session_contract_rejects_caller_identity_as_required_authority(self) -> None:
        document = {
            "components": {
                "securitySchemes": {},
                "schemas": {
                    "AcquireSlotHoldRequest": {
                        "required": ["help_intent_id", "slot_id", "client_identity_id"],
                        "properties": {"client_identity_id": {}},
                    },
                    "CreateCheckoutInstructionRequest": {
                        "required": ["hold_id", "method_code", "client_identity_id"],
                        "properties": {"client_identity_id": {}},
                    },
                    "AccountDeletionRequest": {
                        "required": ["source", "identity_id"],
                        "properties": {"identity_id": {}},
                    },
                },
            },
            "paths": {
                "/v1/slot-holds": {"post": {}},
                "/v1/slot-holds/{id}/checkout-options": {
                    "get": {
                        "parameters": [
                            {
                                "name": "client_identity_id",
                                "in": "query",
                                "required": True,
                            }
                        ]
                    }
                },
                "/v1/checkout-instructions": {"post": {}},
                "/v1/account-deletions": {"post": {}},
                "/v1/bookings/{id}/fulfillment": {
                    "get": {
                        "parameters": [
                            {
                                "name": "identity_id",
                                "in": "query",
                                "required": True,
                            }
                        ]
                    }
                },
            },
        }
        errors = validate_client_session_contract(document)
        self.assertGreaterEqual(len(errors), 10)
        self.assertTrue(any("must require ClientSession" in error for error in errors))
        self.assertTrue(any("must be optional compatibility input" in error for error in errors))

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
