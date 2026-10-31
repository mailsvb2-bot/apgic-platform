\set ON_ERROR_STOP on

-- Dedicated SQL execution proof: branch migration must have been installed.
DO $$
DECLARE
  present_tables integer;
  denied text;
  invalid_blocked boolean := false;
BEGIN
  SELECT count(*) INTO present_tables
    FROM pg_class WHERE oid IN (
      'public.instant_presence'::regclass,
      'public.instant_dispatches'::regclass
    );
  IF present_tables <> 2 THEN
    RAISE EXCEPTION 'R6 Duty/Now canonical tables missing';
  END IF;

  denied := apgic_instant_enable(
    '00000000-0000-0000-0000-00000000b001'::uuid,
    '00000000-0000-0000-0000-00000000d900'::uuid,
    true, now()
  );
  IF denied <> 'DUTY_NOT_ELIGIBLE' THEN
    RAISE EXCEPTION 'Duty availability bypassed verified Product/Qualification: %', denied;
  END IF;

  denied := apgic_instant_request(
    '00000000-0000-0000-0000-00000000d101'::uuid,
    '00000000-0000-0000-0000-00000000d102'::uuid,
    '00000000-0000-0000-0000-00000000b002'::uuid,
    '00000000-0000-0000-0000-00000000d103'::uuid,
    now()
  );
  IF denied <> 'DUTY_INTENT_NOT_ELIGIBLE' THEN
    RAISE EXCEPTION 'Duty dispatch bypassed confirmed HelpIntent ownership: %', denied;
  END IF;

  denied := apgic_instant_answer(
    '00000000-0000-0000-0000-00000000d101'::uuid,
    '00000000-0000-0000-0000-00000000b001'::uuid,
    true, now()
  );
  IF denied <> 'DUTY_NOT_FOUND' THEN
    RAISE EXCEPTION 'Duty answer accepted forged offer id: %', denied;
  END IF;

  BEGIN
    INSERT INTO instant_dispatches (
      id, client_identity_id, request_key, help_intent_id, state
    ) VALUES (
      '00000000-0000-0000-0000-00000000d190',
      '00000000-0000-0000-0000-00000000b002',
      '00000000-0000-0000-0000-00000000d191',
      '00000000-0000-0000-0000-00000000d193',
      'ACCEPTED'
    );
  EXCEPTION WHEN check_violation THEN
    invalid_blocked := true;
  WHEN foreign_key_violation THEN
    -- Unexpected foreign-key miss must not mask a missing CHECK.
    RAISE EXCEPTION 'Duty accepted-state CHECK not proven; fixture FK missing';
  END;
  IF NOT invalid_blocked THEN
    RAISE EXCEPTION 'R6 ACCEPTED dispatch without priced offer was persisted';
  END IF;
END
$$;

-- Verify indexes / stored function contracts rather than reading mutable
-- client-side state. No live provider or unverified specialist is fabricated.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE tablename='instant_dispatches'
    AND indexname='instant_one_active_dispatch_per_specialist'
  ) THEN
    RAISE EXCEPTION 'R6 unique active specialist dispatch index missing';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE tablename='instant_dispatches'
    AND indexname='instant_one_active_dispatch_per_client'
  ) THEN
    RAISE EXCEPTION 'R6 unique active client dispatch index missing';
  END IF;
END
$$;
