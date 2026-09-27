from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tools.payment_taxonomy_architecture_guard import validate_payment_taxonomy


def write_fixture(root: Path, rel: str, content: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def valid_fixture(root: Path) -> None:
    write_fixture(
        root,
        "backend/internal/payments/taxonomy.go",
        '''package payments
type ProviderID string
type MethodCode string
type RailCode string
type Selection struct {
    ProviderID ProviderID `json:"provider_id"`
    MethodCode MethodCode `json:"method_code"`
    RailCode RailCode `json:"rail_code"`
}
''',
    )
    write_fixture(
        root,
        "contracts/openapi/apgic-v1.yaml",
        '''PaymentSelection:
  required: [provider_id, method_code, rail_code]
  properties:
    provider_id:
      type: string
    method_code:
      type: string
    rail_code:
      type: string
''',
    )
    write_fixture(
        root,
        "backend/migrations/000009_r2_payment_control_plane.sql",
        '''CREATE TABLE payment_provider_config_versions (
  provider_config_id uuid,
  method_codes text[],
  rail_codes text[]
);
CREATE TABLE payment_routing_decisions (
  provider_config_id uuid,
  selected_method_code text,
  selected_rail_code text
);
''',
    )


class PaymentTaxonomyArchitectureGuardTests(unittest.TestCase):
    def test_accepts_separate_provider_method_and_rail(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            self.assertEqual(validate_payment_taxonomy(root), [])

    def test_rejects_mixed_payment_type_in_production_source(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write_fixture(root, "apps/web/payment.ts", "export const payment_type = 'bank_card';\n")
            errors = validate_payment_taxonomy(root)
            self.assertEqual(len(errors), 1)
            self.assertIn("mixed payment_type semantics are forbidden", errors[0])

    def test_rejects_missing_storage_taxonomy_anchor(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            migration = root / "backend/migrations/000009_r2_payment_control_plane.sql"
            migration.write_text(migration.read_text(encoding="utf-8").replace("selected_rail_code text", "rail text"), encoding="utf-8")
            errors = validate_payment_taxonomy(root)
            self.assertTrue(any("selected_rail_code" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
