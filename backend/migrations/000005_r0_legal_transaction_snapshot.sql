BEGIN;

CREATE TABLE legal_transaction_snapshots (
  id uuid PRIMARY KEY,
  transaction_ref text NOT NULL UNIQUE CHECK (btrim(transaction_ref) <> ''),
  seller_or_service_provider_id text NOT NULL CHECK (btrim(seller_or_service_provider_id) <> ''),
  commercial_owner_id text NOT NULL CHECK (btrim(commercial_owner_id) <> ''),
  payment_recipient_id text NOT NULL CHECK (btrim(payment_recipient_id) <> ''),
  platform_role text NOT NULL CHECK (btrim(platform_role) <> ''),
  fiscal_responsibility_id text NOT NULL CHECK (btrim(fiscal_responsibility_id) <> ''),
  refund_responsibility_id text NOT NULL CHECK (btrim(refund_responsibility_id) <> ''),
  payout_beneficiary_id text NOT NULL CHECK (btrim(payout_beneficiary_id) <> ''),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  occurred_at timestamptz NOT NULL
);

CREATE TRIGGER legal_transaction_snapshots_append_only
BEFORE UPDATE OR DELETE ON legal_transaction_snapshots
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
