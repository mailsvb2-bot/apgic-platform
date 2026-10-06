from __future__ import annotations

import unittest

from tools.auth_contract_guard import (
    validate_authorization_contract,
    validate_client_session_contract,
    validate_auth001_web_surface_proof,
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
                "/v1/help-intents/{id}/confirm": {"post": {"security": [{"ClientSession": []}]}},
                "/v1/help-intents/{id}/matches": {"get": {"security": [{"ClientSession": []}]}},
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
                "/v1/legal-acceptances": {"post": {"security": [{"ClientSession": []}]}},
                "/v1/legal-acceptances/{documentID}/{documentVersion}": {"get": {"security": [{"ClientSession": []}]}},
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
                "/v1/help-intents/{id}/confirm": {"post": {}},
                "/v1/help-intents/{id}/matches": {"get": {}},
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
                "/v1/legal-acceptances": {"post": {}},
                "/v1/legal-acceptances/{documentID}/{documentVersion}": {"get": {}},
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
        self.assertGreaterEqual(len(errors), 12)
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


    def test_auth001_web_surface_proof_requires_proxy_denial_and_audit(self) -> None:
        script = r'''
APGIC_API_ORIGIN="$API_ORIGIN"
npm run start -- --hostname 127.0.0.1
/v1/organizations/${org_b}/private-profile
X-Organization-Context: $org_a
X-Organization-Context: $org_b
AUTH_CROSS_TENANT_DENY
AUTH_TENANT_CONTEXT_DENIED
TOP SECRET ORGANIZATION B
FROM audit_records
"web_proxy_exercised": true
"postgres_persistence_exercised": true
"private_resource_disclosed": false
"audit_evidence_persisted": true
'''
        workflow = '''
Prove AUTH-001 tenant isolation through WEB proxy
bash tools/auth001_web_tenant_isolation_e2e.sh
auth001-web-tenant-isolation
evidence/auth001-web-tenant-isolation.json
'''
        self.assertEqual(validate_auth001_web_surface_proof(script, workflow), [])

    def test_auth001_web_surface_proof_rejects_missing_cross_tenant_reason(self) -> None:
        script = r'''
APGIC_API_ORIGIN="$API_ORIGIN"
npm run start -- --hostname 127.0.0.1
/v1/organizations/${org_b}/private-profile
X-Organization-Context: $org_a
X-Organization-Context: $org_b
AUTH_TENANT_CONTEXT_DENIED
TOP SECRET ORGANIZATION B
FROM audit_records
"web_proxy_exercised": true
"postgres_persistence_exercised": true
"private_resource_disclosed": false
"audit_evidence_persisted": true
'''
        workflow = '''
Prove AUTH-001 tenant isolation through WEB proxy
bash tools/auth001_web_tenant_isolation_e2e.sh
auth001-web-tenant-isolation
evidence/auth001-web-tenant-isolation.json
'''
        errors = validate_auth001_web_surface_proof(script, workflow)
        self.assertTrue(any("AUTH_CROSS_TENANT_DENY" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
