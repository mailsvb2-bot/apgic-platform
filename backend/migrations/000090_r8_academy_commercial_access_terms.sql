-- Academy author controls course visibility and price; Commerce keeps the
-- single commercial product identity. A curriculum publication never collects
-- funds or silently publishes its associated Commerce product.
CREATE TABLE commerce_product_access_terms (
  product_id uuid PRIMARY KEY REFERENCES products(id),
  access_mode text NOT NULL CHECK (access_mode IN ('FREE','PAID')),
  price_minor bigint NOT NULL CHECK (price_minor BETWEEN 0 AND 10000000000),
  currency text NOT NULL DEFAULT 'RUB' CHECK (currency='RUB'),
  changed_by uuid NOT NULL REFERENCES identities(id),
  changed_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
  CHECK ((access_mode='FREE' AND price_minor=0)
     OR (access_mode='PAID' AND price_minor>0))
);
CREATE FUNCTION commerce_product_access_terms_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM academy_courses c WHERE c.product_id=NEW.product_id)
 THEN RAISE EXCEPTION 'course terms require one canonical Academy product'; END IF;
 IF TG_OP='UPDATE' THEN
   IF NEW.product_id<>OLD.product_id OR NEW.revision<>OLD.revision+1
     OR NEW.changed_at<OLD.changed_at THEN
     RAISE EXCEPTION 'course commercial terms product/revision must be preserved';
   END IF;
 END IF;
 RETURN NEW;
END;
$fn$;
CREATE TRIGGER commerce_product_access_terms_guard
BEFORE INSERT OR UPDATE ON commerce_product_access_terms
FOR EACH ROW EXECUTE FUNCTION commerce_product_access_terms_guard();
CREATE TRIGGER commerce_product_access_terms_no_delete
BEFORE DELETE ON commerce_product_access_terms FOR EACH ROW EXECUTE FUNCTION apgic_reject_delete();

-- First-class non-cash FREE learning entitlement. Unlike a purchase,
-- this is a non-payment access grant: no attempt, capture, order or
-- transaction verification is generated or accepted as its basis.
ALTER TABLE store_entitlements DROP CONSTRAINT store_entitlements_kind_check;
ALTER TABLE store_entitlements ADD CONSTRAINT store_entitlements_kind_check
  CHECK(kind IN ('ONE_TIME','SUBSCRIPTION','FREE'));
ALTER TABLE store_entitlements ALTER COLUMN verification_id DROP NOT NULL;
ALTER TABLE store_entitlements ADD CONSTRAINT store_entitlements_free_basis_check
  CHECK ((kind='FREE' AND verification_id IS NULL)
     OR (kind<>'FREE' AND verification_id IS NOT NULL));
CREATE UNIQUE INDEX store_entitlements_free_per_product_identity_idx
ON store_entitlements(identity_id,product_ref) WHERE kind='FREE';

CREATE OR REPLACE FUNCTION apgic_store_entitlement_insert_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE verification store_transaction_verifications%ROWTYPE;
BEGIN
 IF NEW.kind='FREE' THEN
   IF NEW.verification_id IS NOT NULL OR NOT EXISTS(
     SELECT 1 FROM academy_courses c
     JOIN academy_course_versions v ON v.course_id=c.id AND v.state='PUBLISHED'
     JOIN commerce_product_access_terms terms ON terms.product_id=c.product_id
     WHERE c.product_id::text=NEW.product_ref AND c.state='PUBLISHED'
       AND terms.access_mode='FREE' AND terms.price_minor=0
   ) THEN RAISE EXCEPTION 'free enrollment requires published zero-price course offer'; END IF;
 ELSE
   SELECT * INTO verification FROM store_transaction_verifications WHERE id=NEW.verification_id;
   IF NOT FOUND OR verification.identity_id<>NEW.identity_id
     OR verification.product_ref<>NEW.product_ref
     OR verification.entitlement_kind<>NEW.kind
   THEN RAISE EXCEPTION 'store entitlement must match verified transaction'; END IF;
 END IF;
 IF NEW.state<>'ACTIVE' OR NEW.updated_at<>NEW.activated_at
 THEN RAISE EXCEPTION 'new store entitlement must start ACTIVE'; END IF;
 RETURN NEW;
END;
$fn$;

-- Reuse the existing immutable entitlement and revoke lifecycle. A free
-- grant remains valid after a subsequent price change until explicitly
-- revoked; a price change is never retroactive payment collection.
CREATE OR REPLACE FUNCTION academy_entitlement_is_active(p_entitlement uuid)
RETURNS boolean LANGUAGE sql STABLE AS $academy_entitlement_access$
 SELECT EXISTS(
  SELECT 1 FROM store_entitlements t
  WHERE t.id=p_entitlement AND t.kind='FREE'
    AND t.state='ACTIVE' AND t.verification_id IS NULL
 ) OR EXISTS(
  SELECT 1 FROM store_entitlements t
  JOIN store_transaction_verifications sv ON sv.id=t.verification_id
   AND sv.identity_id=t.identity_id AND sv.product_ref=t.product_ref
  JOIN orders o ON o.id=sv.order_id AND o.product_id::text=t.product_ref
  JOIN payment_attempts pa ON pa.id=sv.payment_attempt_id
   AND pa.order_id=o.id AND pa.state='SUCCEEDED'
   AND pa.amount_minor=o.amount_minor AND pa.currency=o.currency
  JOIN payment_effects pe ON pe.id=sv.payment_effect_id
   AND pe.attempt_id=pa.id AND pe.effect_kind='CAPTURED'
   AND pe.ledger_entry_id=sv.ledger_entry_id
  JOIN ledger_entries le ON le.id=sv.ledger_entry_id
   AND le.provider_evidence_ref=pe.provider_evidence_ref
  WHERE t.id=p_entitlement AND t.state='ACTIVE' AND t.kind<>'FREE'
   AND NOT EXISTS(SELECT 1 FROM store_subscription_projections projection
    WHERE projection.entitlement_id=t.id
      AND (projection.identity_id IS DISTINCT FROM t.identity_id
       OR projection.entitlement_state<>'ACTIVE'))
 );
$academy_entitlement_access$;

CREATE OR REPLACE FUNCTION academy_enrollment_entitlement_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE ent store_entitlements%ROWTYPE; product_ref text; version_state text;
BEGIN
 SELECT * INTO ent FROM store_entitlements WHERE id=NEW.entitlement_id;
 SELECT c.product_id::text,v.state INTO product_ref,version_state
 FROM academy_course_versions v JOIN academy_courses c ON c.id=v.course_id
 WHERE v.id=NEW.version_id;
 IF NOT FOUND OR ent.id IS NULL OR ent.identity_id<>NEW.identity_id
   OR NOT academy_entitlement_is_active(ent.id)
   OR ent.product_ref<>product_ref OR version_state<>'PUBLISHED'
 THEN RAISE EXCEPTION 'academy enrollment requires active entitlement for published version'; END IF;
 IF ent.kind='FREE' THEN
   IF ent.verification_id IS NOT NULL OR NOT EXISTS(
    SELECT 1 FROM commerce_product_access_terms terms
    WHERE terms.product_id::text=ent.product_ref AND terms.access_mode='FREE'
      AND terms.price_minor=0
   ) THEN RAISE EXCEPTION 'free enrollment requires current free offer'; END IF;
 ELSIF NOT EXISTS(
   SELECT 1 FROM store_transaction_verifications verification
   JOIN orders order_row ON order_row.id=verification.order_id
   WHERE verification.id=ent.verification_id
     AND verification.identity_id=ent.identity_id
     AND verification.product_ref=ent.product_ref
     AND order_row.product_id::text=ent.product_ref
 ) THEN RAISE EXCEPTION 'paid enrollment requires verified external purchase'; END IF;
 RETURN NEW;
END;
$fn$;
