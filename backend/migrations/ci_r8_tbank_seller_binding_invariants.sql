\set ON_ERROR_STOP on
-- Real PostgreSQL checks of the canonical per-owner T-Bank authorization.
-- Fixture connector is not a bank; no PSP requests or Entitlements are issued.
DO $proof$
DECLARE
 owner_ref text;
 approver uuid;
 blocked boolean;
BEGIN
 SELECT lower(owner_type)||'/'||owner_id::text INTO owner_ref
 FROM products WHERE owner_id IS NOT NULL ORDER BY id LIMIT 1;
 SELECT id INTO approver FROM identities ORDER BY id LIMIT 1;
 IF owner_ref IS NULL OR approver IS NULL THEN
  RAISE EXCEPTION 'bank seller proof requires existing Commerce owner and approver';
 END IF;
 blocked:=false;
 BEGIN
  INSERT INTO academy_bank_seller_bindings(
   id,product_owner_ref,provider_config_id,terminal_key,
   credential_version_ref,approval_evidence_ref,approved_by,approved_at)
  VALUES(
   '00000000-0000-0000-0000-00000000e901',owner_ref,
   '00000000-0000-0000-0000-00000000d102',
   'ci-foreign-provider','secret-version/ci-payment-b/v1',
   'ci-approval/wrong-connector',approver,now());
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'non-TBank connector was bound as Academy merchant'; END IF;

 INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
 VALUES ('00000000-0000-0000-0000-00000000e911',
  'PAYMENT_PROVIDER','TBANK','ACTIVE','secret://ci/r8-tbank-terminal');
 INSERT INTO payment_provider_config_versions(
  id,provider_instance_id,config_version,status,priority,manifest_version,
  certification_evidence_refs,jurisdiction_codes,currencies,method_codes,
  rail_codes,execution_owner,credential_version_ref,effective_from)
 SELECT '00000000-0000-0000-0000-00000000e912'::uuid,
  '00000000-0000-0000-0000-00000000e911'::uuid,
  'ci-r8-bank-v1','ACTIVE',priority,manifest_version,
  certification_evidence_refs,jurisdiction_codes,currencies,method_codes,
  rail_codes,execution_owner,'secret-version/ci-r8-tbank/1',now()
 FROM payment_provider_config_versions
 WHERE id='00000000-0000-0000-0000-00000000d102'::uuid;

 INSERT INTO academy_bank_seller_bindings(
  id,product_owner_ref,provider_config_id,terminal_key,
  credential_version_ref,approval_evidence_ref,approved_by,approved_at)
 VALUES('00000000-0000-0000-0000-00000000e913',owner_ref,
  '00000000-0000-0000-0000-00000000e912',
  'ci-seller-terminal',
  'secret-version/ci-r8-tbank/1','ci-approval/owner-verified',approver,now());
 IF NOT EXISTS(
  SELECT 1 FROM academy_bank_seller_bindings
  WHERE product_owner_ref=owner_ref AND terminal_key='ci-seller-terminal'
    AND revoked_at IS NULL
 ) THEN RAISE EXCEPTION 'verified merchant binding unavailable'; END IF;
 blocked:=false;
 BEGIN
  UPDATE academy_bank_seller_bindings
  SET terminal_key='attacker-terminal'
  WHERE id='00000000-0000-0000-0000-00000000e913';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'merchant terminal changed without reapproval'; END IF;
 blocked:=false;
 BEGIN
  INSERT INTO academy_bank_seller_bindings(
   id,product_owner_ref,provider_config_id,terminal_key,
   credential_version_ref,approval_evidence_ref,approved_by,approved_at)
  VALUES('00000000-0000-0000-0000-00000000e914',owner_ref,
   '00000000-0000-0000-0000-00000000e912','another-seller-terminal',
   'secret-version/ci-r8-tbank/1','ci-approval/duplicate',approver,now());
 EXCEPTION WHEN unique_violation THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'seller had two simultaneous active bank terminal bindings'; END IF;
 UPDATE academy_bank_seller_bindings
 SET revoked_at=now()+interval '1 second'
 WHERE id='00000000-0000-0000-0000-00000000e913';
 blocked:=false;
 BEGIN
  UPDATE academy_bank_seller_bindings
  SET revoked_at=now()+interval '2 second'
  WHERE id='00000000-0000-0000-0000-00000000e913';
 EXCEPTION WHEN raise_exception THEN blocked:=true;
 END;
 IF NOT blocked THEN RAISE EXCEPTION 'seller revocation could be overwritten'; END IF;
 IF EXISTS(SELECT 1 FROM academy_bank_seller_bindings
           WHERE product_owner_ref=owner_ref AND revoked_at IS NULL)
 THEN RAISE EXCEPTION 'revoked merchant still routable'; END IF;
END
$proof$;
