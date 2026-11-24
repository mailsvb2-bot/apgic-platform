-- Seller-to-T-Bank terminal authorization is a Commerce configuration boundary,
-- NOT a second order/payment/entitlement runtime. No seller is enabled by
-- default. Only operator-approved immutable bindings can route bank checkout.
CREATE TABLE academy_bank_seller_bindings (
 id uuid PRIMARY KEY,
 product_owner_ref text NOT NULL CHECK (
   product_owner_ref = btrim(product_owner_ref) AND
   product_owner_ref ~ '^[a-z_]+/[0-9a-f-]{36}$'),
 provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
 terminal_key text NOT NULL CHECK (
   terminal_key = btrim(terminal_key) AND
   length(terminal_key) BETWEEN 1 AND 128),
 credential_version_ref text NOT NULL CHECK (
   length(btrim(credential_version_ref)) > 0),
 approved_config_sha256 text NOT NULL CHECK (
   approved_config_sha256 ~ '^[0-9a-f]{64}
 approval_evidence_ref text NOT NULL CHECK (
   length(btrim(approval_evidence_ref)) > 0),
 approved_by uuid NOT NULL REFERENCES identities(id),
 approved_at timestamptz NOT NULL,
 revoked_at timestamptz,
 CHECK (revoked_at IS NULL OR revoked_at >= approved_at)
);
CREATE UNIQUE INDEX academy_bank_seller_active_owner
 ON academy_bank_seller_bindings(product_owner_ref) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX academy_bank_seller_active_terminal
 ON academy_bank_seller_bindings(terminal_key) WHERE revoked_at IS NULL;

CREATE OR REPLACE FUNCTION apgic_academy_bank_seller_binding_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'UPDATE' THEN
   IF OLD.revoked_at IS NOT NULL OR NEW.revoked_at IS NULL
      OR NEW.id IS DISTINCT FROM OLD.id
      OR NEW.product_owner_ref IS DISTINCT FROM OLD.product_owner_ref
      OR NEW.provider_config_id IS DISTINCT FROM OLD.provider_config_id
      OR NEW.terminal_key IS DISTINCT FROM OLD.terminal_key
      OR NEW.credential_version_ref IS DISTINCT FROM OLD.credential_version_ref
      OR NEW.approved_config_sha256 IS DISTINCT FROM OLD.approved_config_sha256
      OR NEW.approval_evidence_ref IS DISTINCT FROM OLD.approval_evidence_ref
      OR NEW.approved_by IS DISTINCT FROM OLD.approved_by
      OR NEW.approved_at IS DISTINCT FROM OLD.approved_at
   THEN RAISE EXCEPTION 'bank seller authorization is immutable except one-way revocation'; END IF;
   RETURN NEW;
 END IF;
 IF NOT EXISTS (
   SELECT 1 FROM payment_provider_config_versions cfg
   JOIN connector_instances ci ON ci.id = cfg.provider_instance_id
   WHERE cfg.id = NEW.provider_config_id
     AND cfg.credential_version_ref = NEW.credential_version_ref
     AND cfg.status = 'ACTIVE'
     AND ci.status = 'ACTIVE'
     AND ci.capability_class = 'PAYMENT_PROVIDER'
     AND ci.provider_kind = 'TBANK'
 ) THEN RAISE EXCEPTION 'bank seller requires certified T-Bank provider and exact credential version'; END IF;
 IF NOT EXISTS (
   SELECT 1 FROM products p
   WHERE lower(p.owner_type)||'/'||p.owner_id::text = NEW.product_owner_ref
 ) THEN RAISE EXCEPTION 'bank seller authorization requires an existing canonical product owner'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER academy_bank_seller_bindings_guard
 BEFORE INSERT OR UPDATE ON academy_bank_seller_bindings
 FOR EACH ROW EXECUTE FUNCTION apgic_academy_bank_seller_binding_guard();
CREATE TRIGGER academy_bank_seller_bindings_no_delete
 BEFORE DELETE ON academy_bank_seller_bindings
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
),
 approval_evidence_ref text NOT NULL CHECK (
   length(btrim(approval_evidence_ref)) > 0),
 approved_by uuid NOT NULL REFERENCES identities(id),
 approved_at timestamptz NOT NULL,
 revoked_at timestamptz,
 CHECK (revoked_at IS NULL OR revoked_at >= approved_at)
);
CREATE UNIQUE INDEX academy_bank_seller_active_owner
 ON academy_bank_seller_bindings(product_owner_ref) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX academy_bank_seller_active_terminal
 ON academy_bank_seller_bindings(terminal_key) WHERE revoked_at IS NULL;

CREATE OR REPLACE FUNCTION apgic_academy_bank_seller_binding_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'UPDATE' THEN
   IF OLD.revoked_at IS NOT NULL OR NEW.revoked_at IS NULL
      OR NEW.id IS DISTINCT FROM OLD.id
      OR NEW.product_owner_ref IS DISTINCT FROM OLD.product_owner_ref
      OR NEW.provider_config_id IS DISTINCT FROM OLD.provider_config_id
      OR NEW.terminal_key IS DISTINCT FROM OLD.terminal_key
      OR NEW.credential_version_ref IS DISTINCT FROM OLD.credential_version_ref
      OR NEW.approval_evidence_ref IS DISTINCT FROM OLD.approval_evidence_ref
      OR NEW.approved_by IS DISTINCT FROM OLD.approved_by
      OR NEW.approved_at IS DISTINCT FROM OLD.approved_at
   THEN RAISE EXCEPTION 'bank seller authorization is immutable except one-way revocation'; END IF;
   RETURN NEW;
 END IF;
 IF NOT EXISTS (
   SELECT 1 FROM payment_provider_config_versions cfg
   JOIN connector_instances ci ON ci.id = cfg.provider_instance_id
   WHERE cfg.id = NEW.provider_config_id
     AND cfg.credential_version_ref = NEW.credential_version_ref
     AND cfg.status = 'ACTIVE'
     AND ci.status = 'ACTIVE'
     AND ci.capability_class = 'PAYMENT_PROVIDER'
     AND ci.provider_kind = 'TBANK'
 ) THEN RAISE EXCEPTION 'bank seller requires certified T-Bank provider and exact credential version'; END IF;
 IF NOT EXISTS (
   SELECT 1 FROM products p
   WHERE lower(p.owner_type)||'/'||p.owner_id::text = NEW.product_owner_ref
 ) THEN RAISE EXCEPTION 'bank seller authorization requires an existing canonical product owner'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER academy_bank_seller_bindings_guard
 BEFORE INSERT OR UPDATE ON academy_bank_seller_bindings
 FOR EACH ROW EXECUTE FUNCTION apgic_academy_bank_seller_binding_guard();
CREATE TRIGGER academy_bank_seller_bindings_no_delete
 BEFORE DELETE ON academy_bank_seller_bindings
 FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();
