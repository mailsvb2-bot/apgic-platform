\set ON_ERROR_STOP on
-- A direct Academy refund never fabricates a consultation booking; existing
-- consultation cancellation and evidence guards remain active.
DO $proof$
DECLARE
  function_sql text;
  sample orders%ROWTYPE;
  refused boolean := false;
BEGIN
  SELECT pg_get_functiondef('apgic_refund_request_insert_guard()'::regprocedure) INTO function_sql;
  IF function_sql NOT LIKE '%direct bank refund requires entire verified one-time purchase%'
     OR function_sql NOT LIKE '%refund requires a cancelled booking%' THEN
    RAISE EXCEPTION 'refund request guard lost either direct-sale or booking safety';
  END IF;
  SELECT pg_get_functiondef('apgic_refund_request_transition_guard()'::regprocedure) INTO function_sql;
  IF function_sql NOT LIKE '%NEW.booking_id IS DISTINCT FROM OLD.booking_id%' THEN
    RAISE EXCEPTION 'nullable direct booking identity is not immutable';
  END IF;
  SELECT pg_get_functiondef(
    'apgic_apply_refund_success(uuid,uuid,text,uuid,uuid,timestamp with time zone)'::regprocedure
  ) INTO function_sql;
  IF function_sql NOT LIKE '%UPDATE store_entitlements entitlement%'
     OR function_sql NOT LIKE '%current_refund.booking_id IS NULL%'
     OR function_sql NOT LIKE '%INSERT INTO refund_effects%' THEN
    RAISE EXCEPTION 'direct refund missing atomic receipt/ledger/entitlement revocation';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name='refund_requests' AND column_name='booking_id'
               AND is_nullable='NO') THEN
    RAISE EXCEPTION 'direct Academy refund still requires a synthetic booking';
  END IF;
  SELECT * INTO sample FROM orders
   WHERE id='00000000-0000-0000-0000-00000000b501'::uuid;
  IF sample.booking_id IS NULL THEN
    RAISE EXCEPTION 'existing consultation order lost booking association';
  END IF;
  -- No genuine captured purchase or legal approval was supplied by this
  -- fixture. The refund insert must be rejected by the same canonical guard.
  BEGIN
    INSERT INTO refund_requests(
     id,booking_id,order_id,original_payment_attempt_id,
     original_provider_config_id,amount_minor,currency,policy_version,
     policy_decision,reason_code,idempotency_key,execution_owner,state,
     created_at,updated_at)
    SELECT '00000000-0000-0000-0000-00000000b999'::uuid,NULL,o.id,
     a.id,d.provider_config_id,a.amount_minor,a.currency,
     'unauthorized-test','ALLOW','unverified-direct-refund',
     'direct-refund-denied','EXTERNAL_PROVIDER','REQUESTED',now(),now()
    FROM orders o JOIN payment_attempts a ON a.order_id=o.id
    JOIN payment_routing_decisions d ON d.id=a.routing_decision_id
    WHERE o.id=sample.id LIMIT 1;
    IF FOUND THEN
      RAISE EXCEPTION 'unauthorized direct refund inserted';
    END IF;
  EXCEPTION WHEN raise_exception THEN
    refused := true;
  END;
  IF NOT refused THEN
    RAISE EXCEPTION 'non-Academy bookingless refund was not rejected';
  END IF;
END
$proof$;
