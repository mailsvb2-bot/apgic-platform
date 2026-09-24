BEGIN;

CREATE TABLE store_policy_snapshots (
  id uuid PRIMARY KEY,
  policy_version text NOT NULL UNIQUE CHECK (btrim(policy_version) <> ''),
  effective_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE store_policy_rules (
  id uuid PRIMARY KEY,
  snapshot_id uuid NOT NULL REFERENCES store_policy_snapshots(id),
  product_type text NOT NULL CHECK (btrim(product_type) <> ''),
  surface text NOT NULL CHECK (surface IN ('IOS','ANDROID')),
  store_code text NOT NULL CHECK (btrim(store_code) <> ''),
  storefront text NOT NULL CHECK (btrim(storefront) <> ''),
  jurisdiction_code text NOT NULL CHECK (btrim(jurisdiction_code) <> ''),
  enabled boolean NOT NULL,
  rail_code text NOT NULL CHECK (btrim(rail_code) <> ''),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  UNIQUE (
    snapshot_id, product_type, surface, store_code, storefront, jurisdiction_code
  ),
  CHECK (
    (enabled AND rail_code <> 'PURCHASE_DISABLED')
    OR
    (NOT enabled AND rail_code = 'PURCHASE_DISABLED')
  )
);

CREATE TRIGGER store_policy_snapshots_append_only
BEFORE UPDATE OR DELETE ON store_policy_snapshots
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TRIGGER store_policy_rules_append_only
BEFORE UPDATE OR DELETE ON store_policy_rules
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE store_commerce_decisions (
  id uuid PRIMARY KEY,
  snapshot_id uuid NOT NULL REFERENCES store_policy_snapshots(id),
  identity_id uuid NOT NULL REFERENCES identities(id),
  product_ref text NOT NULL CHECK (btrim(product_ref) <> ''),
  product_type text NOT NULL CHECK (btrim(product_type) <> ''),
  surface text NOT NULL CHECK (surface IN ('IOS','ANDROID')),
  store_code text NOT NULL CHECK (btrim(store_code) <> ''),
  storefront text NOT NULL CHECK (btrim(storefront) <> ''),
  jurisdiction_code text NOT NULL CHECK (btrim(jurisdiction_code) <> ''),
  outcome text NOT NULL CHECK (outcome IN ('ALLOWED','PURCHASE_DISABLED')),
  rail_code text NOT NULL CHECK (btrim(rail_code) <> ''),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  decided_at timestamptz NOT NULL,
  CHECK (
    (outcome = 'ALLOWED' AND rail_code <> 'PURCHASE_DISABLED')
    OR
    (outcome = 'PURCHASE_DISABLED' AND rail_code = 'PURCHASE_DISABLED')
  )
);

CREATE OR REPLACE FUNCTION apgic_store_commerce_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  snapshot_time timestamptz;
  matched_rule store_policy_rules%ROWTYPE;
BEGIN
  SELECT effective_at
  INTO snapshot_time
  FROM store_policy_snapshots
  WHERE id = NEW.snapshot_id;

  IF NOT FOUND OR snapshot_time > NEW.decided_at THEN
    RAISE EXCEPTION 'store commerce decision requires effective policy snapshot';
  END IF;

  SELECT *
  INTO matched_rule
  FROM store_policy_rules
  WHERE snapshot_id = NEW.snapshot_id
    AND product_type = NEW.product_type
    AND surface = NEW.surface
    AND store_code = NEW.store_code
    AND storefront = NEW.storefront
    AND jurisdiction_code = NEW.jurisdiction_code;

  IF NOT FOUND THEN
    IF NEW.outcome <> 'PURCHASE_DISABLED'
      OR NEW.rail_code <> 'PURCHASE_DISABLED'
      OR NEW.reason_code <> 'STORE_POLICY_NOT_CONFIGURED' THEN
      RAISE EXCEPTION 'unknown store policy path must fail closed';
    END IF;
    RETURN NEW;
  END IF;

  IF matched_rule.enabled THEN
    IF NEW.outcome <> 'ALLOWED'
      OR NEW.rail_code <> matched_rule.rail_code
      OR NEW.reason_code <> matched_rule.reason_code THEN
      RAISE EXCEPTION 'store commerce decision diverges from enabled policy rule';
    END IF;
  ELSE
    IF NEW.outcome <> 'PURCHASE_DISABLED'
      OR NEW.rail_code <> 'PURCHASE_DISABLED'
      OR NEW.reason_code <> matched_rule.reason_code THEN
      RAISE EXCEPTION 'disabled store policy rule must block purchase';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER store_commerce_decisions_insert_guard
BEFORE INSERT ON store_commerce_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_store_commerce_decision_guard();

CREATE TRIGGER store_commerce_decisions_append_only
BEFORE UPDATE OR DELETE ON store_commerce_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE store_transaction_verifications (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  order_id uuid NOT NULL REFERENCES orders(id),
  payment_attempt_id uuid NOT NULL UNIQUE REFERENCES payment_attempts(id),
  payment_effect_id uuid NOT NULL UNIQUE REFERENCES payment_effects(id),
  ledger_entry_id uuid NOT NULL UNIQUE REFERENCES ledger_entries(id),
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  external_transaction_id text NOT NULL CHECK (btrim(external_transaction_id) <> ''),
  product_ref text NOT NULL CHECK (btrim(product_ref) <> ''),
  entitlement_kind text NOT NULL CHECK (entitlement_kind IN ('ONE_TIME','SUBSCRIPTION')),
  policy_snapshot_id uuid NOT NULL REFERENCES store_policy_snapshots(id),
  provider_evidence_ref text NOT NULL CHECK (btrim(provider_evidence_ref) <> ''),
  verified_at timestamptz NOT NULL,
  UNIQUE (provider_instance_id, external_transaction_id)
);

CREATE OR REPLACE FUNCTION apgic_store_transaction_verification_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  attempt payment_attempts%ROWTYPE;
  effect payment_effects%ROWTYPE;
  route payment_routing_decisions%ROWTYPE;
  config payment_provider_config_versions%ROWTYPE;
  booking_client uuid;
BEGIN
  SELECT *
  INTO attempt
  FROM payment_attempts
  WHERE id = NEW.payment_attempt_id;

  IF NOT FOUND OR attempt.order_id <> NEW.order_id OR attempt.state <> 'SUCCEEDED' THEN
    RAISE EXCEPTION 'store verification requires succeeded canonical payment attempt';
  END IF;

  SELECT *
  INTO effect
  FROM payment_effects
  WHERE id = NEW.payment_effect_id;

  IF NOT FOUND
    OR effect.attempt_id <> NEW.payment_attempt_id
    OR effect.effect_kind <> 'CAPTURED'
    OR effect.ledger_entry_id IS DISTINCT FROM NEW.ledger_entry_id THEN
    RAISE EXCEPTION 'store verification requires captured payment effect and ledger basis';
  END IF;

  SELECT *
  INTO route
  FROM payment_routing_decisions
  WHERE id = attempt.routing_decision_id;

  IF NOT FOUND OR route.selected_rail_code <> 'STORE_BILLING' THEN
    RAISE EXCEPTION 'store verification requires STORE_BILLING payment rail';
  END IF;

  SELECT *
  INTO config
  FROM payment_provider_config_versions
  WHERE id = route.provider_config_id;

  IF NOT FOUND OR config.provider_instance_id <> NEW.provider_instance_id THEN
    RAISE EXCEPTION 'store verification provider must match routed payment provider';
  END IF;

  SELECT booking.client_identity_id
  INTO booking_client
  FROM orders order_row
  JOIN bookings booking ON booking.id = order_row.booking_id
  WHERE order_row.id = NEW.order_id;

  IF booking_client IS DISTINCT FROM NEW.identity_id THEN
    RAISE EXCEPTION 'store verification identity must match canonical order booking client';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM ledger_entries
    WHERE id = NEW.ledger_entry_id
      AND provider_evidence_ref = effect.provider_evidence_ref
  ) THEN
    RAISE EXCEPTION 'store verification ledger evidence mismatch';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER store_transaction_verifications_insert_guard
BEFORE INSERT ON store_transaction_verifications
FOR EACH ROW EXECUTE FUNCTION apgic_store_transaction_verification_guard();

CREATE TRIGGER store_transaction_verifications_append_only
BEFORE UPDATE OR DELETE ON store_transaction_verifications
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE store_entitlements (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  product_ref text NOT NULL CHECK (btrim(product_ref) <> ''),
  kind text NOT NULL CHECK (kind IN ('ONE_TIME','SUBSCRIPTION')),
  state text NOT NULL CHECK (state IN ('ACTIVE','EXPIRED','REVOKED')),
  verification_id uuid NOT NULL UNIQUE REFERENCES store_transaction_verifications(id),
  activated_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_store_entitlement_insert_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  verification store_transaction_verifications%ROWTYPE;
BEGIN
  SELECT *
  INTO verification
  FROM store_transaction_verifications
  WHERE id = NEW.verification_id;

  IF NOT FOUND
    OR verification.identity_id <> NEW.identity_id
    OR verification.product_ref <> NEW.product_ref
    OR verification.entitlement_kind <> NEW.kind THEN
    RAISE EXCEPTION 'store entitlement must match verified transaction';
  END IF;

  IF NEW.state <> 'ACTIVE' OR NEW.updated_at <> NEW.activated_at THEN
    RAISE EXCEPTION 'new store entitlement must start ACTIVE';
  END IF;

  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION apgic_store_entitlement_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.identity_id <> OLD.identity_id
    OR NEW.product_ref <> OLD.product_ref
    OR NEW.kind <> OLD.kind
    OR NEW.verification_id <> OLD.verification_id
    OR NEW.activated_at <> OLD.activated_at THEN
    RAISE EXCEPTION 'store entitlement identity/basis are immutable';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'store entitlement updated_at cannot move backwards';
  END IF;

  IF OLD.state = NEW.state THEN
    RETURN NEW;
  END IF;

  IF OLD.state = 'ACTIVE' AND NEW.state IN ('EXPIRED','REVOKED') THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'invalid store entitlement transition: % -> %', OLD.state, NEW.state;
END;
$$;

CREATE TRIGGER store_entitlements_insert_guard
BEFORE INSERT ON store_entitlements
FOR EACH ROW EXECUTE FUNCTION apgic_store_entitlement_insert_guard();

CREATE TRIGGER store_entitlements_transition_guard
BEFORE UPDATE ON store_entitlements
FOR EACH ROW EXECUTE FUNCTION apgic_store_entitlement_transition_guard();

CREATE TRIGGER store_entitlements_no_delete
BEFORE DELETE ON store_entitlements
FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

CREATE TABLE store_subscription_exit_actions (
  id uuid PRIMARY KEY,
  delete_request_id uuid NOT NULL REFERENCES delete_account_requests(id),
  entitlement_id uuid NOT NULL REFERENCES store_entitlements(id),
  action text NOT NULL CHECK (action IN ('MANAGE_EXTERNALLY','NOT_APPLICABLE')),
  management_target text,
  billing_cancelled_by_apgic boolean NOT NULL DEFAULT false CHECK (billing_cancelled_by_apgic = false),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  created_at timestamptz NOT NULL,
  UNIQUE (delete_request_id, entitlement_id),
  CHECK (
    action <> 'MANAGE_EXTERNALLY'
    OR nullif(btrim(management_target), '') IS NOT NULL
  )
);

CREATE OR REPLACE FUNCTION apgic_store_subscription_exit_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  request_identity uuid;
  entitlement store_entitlements%ROWTYPE;
BEGIN
  SELECT identity_id
  INTO request_identity
  FROM delete_account_requests
  WHERE id = NEW.delete_request_id;

  SELECT *
  INTO entitlement
  FROM store_entitlements
  WHERE id = NEW.entitlement_id;

  IF request_identity IS NULL
    OR NOT FOUND
    OR entitlement.identity_id <> request_identity
    OR entitlement.kind <> 'SUBSCRIPTION' THEN
    RAISE EXCEPTION 'store subscription exit must match deletion identity subscription';
  END IF;

  IF entitlement.state = 'ACTIVE' AND NEW.action <> 'MANAGE_EXTERNALLY' THEN
    RAISE EXCEPTION 'active store subscription must expose external management path';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER store_subscription_exit_actions_insert_guard
BEFORE INSERT ON store_subscription_exit_actions
FOR EACH ROW EXECUTE FUNCTION apgic_store_subscription_exit_guard();

CREATE TRIGGER store_subscription_exit_actions_append_only
BEFORE UPDATE OR DELETE ON store_subscription_exit_actions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_delete_request_transition_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.identity_id <> OLD.identity_id OR NEW.source_surface <> OLD.source_surface THEN
    RAISE EXCEPTION 'delete account request identity/source are immutable';
  END IF;

  IF NEW.state <> OLD.state THEN
    IF OLD.state = 'REQUESTED' AND NEW.state = 'IDENTITY_RECONFIRMED' THEN
      NULL;
    ELSIF OLD.state = 'IDENTITY_RECONFIRMED' AND NEW.state = 'RETENTION_CLASSIFIED' THEN
      NULL;
    ELSIF OLD.state = 'RETENTION_CLASSIFIED'
      AND NEW.state = 'PROVIDER_ERASURE_PENDING' THEN
      NULL;
    ELSIF OLD.state = 'PROVIDER_ERASURE_PENDING'
      AND NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
      NULL;
    ELSIF OLD.state = 'WAITING_FOR_LEGAL_HOLD_EXPIRY'
      AND NEW.state IN ('PROVIDER_ERASURE_PENDING','PARTIALLY_RETAINED_WITH_REASON','COMPLETED') THEN
      NULL;
    ELSE
      RAISE EXCEPTION 'invalid delete account state transition: % -> %', OLD.state, NEW.state;
    END IF;
  END IF;

  IF NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED')
    AND EXISTS (
      SELECT 1
      FROM provider_erasure_jobs
      WHERE delete_request_id = OLD.id
        AND state <> 'SUCCEEDED'
    ) THEN
    RAISE EXCEPTION 'delete account cannot reach terminal/retained state before provider erasure evidence';
  END IF;

  IF NEW.state IN ('WAITING_FOR_LEGAL_HOLD_EXPIRY','PARTIALLY_RETAINED_WITH_REASON','COMPLETED')
    AND EXISTS (
      SELECT 1
      FROM store_entitlements entitlement
      WHERE entitlement.identity_id = OLD.identity_id
        AND entitlement.kind = 'SUBSCRIPTION'
        AND entitlement.state = 'ACTIVE'
        AND NOT EXISTS (
          SELECT 1
          FROM store_subscription_exit_actions exit_action
          WHERE exit_action.delete_request_id = OLD.id
            AND exit_action.entitlement_id = entitlement.id
            AND exit_action.action = 'MANAGE_EXTERNALLY'
            AND exit_action.billing_cancelled_by_apgic = false
            AND nullif(btrim(exit_action.management_target), '') IS NOT NULL
        )
    ) THEN
    RAISE EXCEPTION 'active external store subscription requires management exit path before deletion terminal state';
  END IF;

  IF NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'delete account updated_at cannot move backwards';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TABLE device_integrity_evidence (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  installation_id uuid NOT NULL REFERENCES client_installations(id),
  provider_kind text,
  provider_evidence_ref text,
  verdict text NOT NULL CHECK (verdict IN ('VALID','NEGATIVE','UNSUPPORTED','UNAVAILABLE')),
  server_verified boolean NOT NULL,
  observed_at timestamptz NOT NULL,
  CHECK (
    (verdict = 'UNSUPPORTED'
      AND server_verified = false
      AND provider_evidence_ref IS NULL)
    OR
    (verdict IN ('VALID','NEGATIVE')
      AND server_verified = true
      AND nullif(btrim(provider_kind), '') IS NOT NULL
      AND nullif(btrim(provider_evidence_ref), '') IS NOT NULL)
    OR
    (verdict = 'UNAVAILABLE'
      AND nullif(btrim(provider_kind), '') IS NOT NULL)
  )
);

CREATE OR REPLACE FUNCTION apgic_device_integrity_evidence_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM client_installations
    WHERE id = NEW.installation_id
      AND identity_id = NEW.identity_id
  ) THEN
    RAISE EXCEPTION 'device integrity installation/identity mismatch';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER device_integrity_evidence_insert_guard
BEFORE INSERT ON device_integrity_evidence
FOR EACH ROW EXECUTE FUNCTION apgic_device_integrity_evidence_guard();

CREATE TRIGGER device_integrity_evidence_append_only
BEFORE UPDATE OR DELETE ON device_integrity_evidence
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE device_integrity_risk_decisions (
  id uuid PRIMARY KEY,
  evidence_id uuid NOT NULL UNIQUE REFERENCES device_integrity_evidence(id),
  action text NOT NULL CHECK (action IN ('ALLOW','STEP_UP','REVIEW')),
  reason_code text NOT NULL CHECK (btrim(reason_code) <> ''),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  appeal_path text NOT NULL CHECK (btrim(appeal_path) <> ''),
  audit_record_id uuid NOT NULL REFERENCES audit_records(id),
  decided_at timestamptz NOT NULL
);

CREATE OR REPLACE FUNCTION apgic_device_integrity_risk_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  evidence device_integrity_evidence%ROWTYPE;
  audit_row audit_records%ROWTYPE;
BEGIN
  SELECT *
  INTO evidence
  FROM device_integrity_evidence
  WHERE id = NEW.evidence_id;

  SELECT *
  INTO audit_row
  FROM audit_records
  WHERE id = NEW.audit_record_id;

  IF NOT FOUND
    OR audit_row.action <> 'device_integrity.risk_decision'
    OR audit_row.policy_version <> NEW.policy_version
    OR audit_row.reason <> NEW.reason_code
    OR audit_row.resource_ref <> 'identity/' || evidence.identity_id::text THEN
    RAISE EXCEPTION 'device integrity risk decision requires matching audit evidence';
  END IF;

  IF evidence.verdict = 'VALID' AND NEW.action <> 'ALLOW' THEN
    RAISE EXCEPTION 'valid integrity evidence must not escalate without another risk input';
  END IF;

  IF evidence.verdict = 'NEGATIVE' AND NEW.action NOT IN ('STEP_UP','REVIEW') THEN
    RAISE EXCEPTION 'negative integrity evidence cannot become automatic deny';
  END IF;

  IF evidence.verdict IN ('UNSUPPORTED','UNAVAILABLE') AND NEW.action <> 'REVIEW' THEN
    RAISE EXCEPTION 'unsupported integrity evidence requires graceful review path';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER device_integrity_risk_decisions_insert_guard
BEFORE INSERT ON device_integrity_risk_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_device_integrity_risk_decision_guard();

CREATE TRIGGER device_integrity_risk_decisions_append_only
BEFORE UPDATE OR DELETE ON device_integrity_risk_decisions
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TABLE noncash_entitlement_entries (
  id uuid PRIMARY KEY,
  account_ref text NOT NULL CHECK (btrim(account_ref) <> ''),
  unit_kind text NOT NULL CHECK (
    unit_kind IN ('CREDITS','ORGANIZATION_BUDGET','GROWTH_BUDGET')
  ),
  event_kind text NOT NULL CHECK (event_kind IN ('GRANT','SPEND','EXPIRE')),
  units bigint NOT NULL CHECK (units > 0),
  source_ref text NOT NULL CHECK (btrim(source_ref) <> ''),
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  occurred_at timestamptz NOT NULL,
  UNIQUE (account_ref, unit_kind, idempotency_key)
);

CREATE OR REPLACE FUNCTION apgic_noncash_entitlement_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  available bigint;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtext(NEW.account_ref || ':' || NEW.unit_kind));

  SELECT coalesce(sum(
    CASE
      WHEN event_kind = 'GRANT' THEN units
      WHEN event_kind IN ('SPEND','EXPIRE') THEN -units
      ELSE 0
    END
  ), 0)
  INTO available
  FROM noncash_entitlement_entries
  WHERE account_ref = NEW.account_ref
    AND unit_kind = NEW.unit_kind;

  IF NEW.event_kind IN ('SPEND','EXPIRE') AND NEW.units > available THEN
    RAISE EXCEPTION 'insufficient non-cash entitlement units';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER noncash_entitlement_entries_insert_guard
BEFORE INSERT ON noncash_entitlement_entries
FOR EACH ROW EXECUTE FUNCTION apgic_noncash_entitlement_guard();

CREATE TRIGGER noncash_entitlement_entries_append_only
BEFORE UPDATE OR DELETE ON noncash_entitlement_entries
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
