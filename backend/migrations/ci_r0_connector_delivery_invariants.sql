\set ON_ERROR_STOP on

INSERT INTO connector_instances (
  id, capability_class, provider_kind, status, config_ref
) VALUES (
  '00000000-0000-0000-0000-00000000c160',
  'COMMUNICATION_PROVIDER',
  'ci-connector-delivery',
  'ACTIVE',
  'secretref://ci/connector-delivery'
);

DO $$
BEGIN
  IF (
    SELECT execute_scope
    FROM connector_instances
    WHERE id = '00000000-0000-0000-0000-00000000c160'
  ) <> 'connector:execute:COMMUNICATION_PROVIDER' THEN
    RAISE EXCEPTION 'connector execute scope is not canonical';
  END IF;
END
$$;

DO $$
DECLARE
  r1 text;
  rd text;
  r2 text;
  promoted text;
  conflict_blocked boolean := false;
  rewind_blocked boolean := false;
  delete_blocked boolean := false;
BEGIN
  r1 := apgic_register_connector_delivery(
    '00000000-0000-0000-0000-00000000c160',
    'event-1', 'stream-a', 1,
    repeat('a', 64), 'key-1'
  );
  IF r1 <> 'APPLY' THEN RAISE EXCEPTION 'sequence 1 expected APPLY, got %', r1; END IF;

  IF apgic_register_connector_delivery(
    '00000000-0000-0000-0000-00000000c160',
    'event-1', 'stream-a', 1,
    repeat('a', 64), 'key-1'
  ) <> 'DUPLICATE' THEN
    RAISE EXCEPTION 'duplicate connector event was not idempotent';
  END IF;

  rd := apgic_register_connector_delivery(
    '00000000-0000-0000-0000-00000000c160',
    'event-3', 'stream-a', 3,
    repeat('c', 64), 'key-1'
  );
  IF rd <> 'DEFER' THEN RAISE EXCEPTION 'out-of-order sequence expected DEFER, got %', rd; END IF;

  r2 := apgic_register_connector_delivery(
    '00000000-0000-0000-0000-00000000c160',
    'event-2', 'stream-a', 2,
    repeat('b', 64), 'key-1'
  );
  IF r2 <> 'APPLY' THEN RAISE EXCEPTION 'sequence 2 expected APPLY, got %', r2; END IF;

  promoted := apgic_promote_next_connector_delivery(
    '00000000-0000-0000-0000-00000000c160', 'stream-a'
  );
  IF promoted <> 'event-3' THEN RAISE EXCEPTION 'deferred event was not promoted: %', promoted; END IF;

  BEGIN
    PERFORM apgic_register_connector_delivery(
      '00000000-0000-0000-0000-00000000c160',
      'event-1', 'stream-a', 1,
      repeat('f', 64), 'key-1'
    );
  EXCEPTION WHEN raise_exception THEN
    conflict_blocked := true;
  END;

  BEGIN
    UPDATE connector_stream_positions
    SET last_applied_sequence = 1
    WHERE connector_instance_id = '00000000-0000-0000-0000-00000000c160'
      AND stream_id = 'stream-a';
  EXCEPTION WHEN raise_exception THEN
    rewind_blocked := true;
  END;

  BEGIN
    DELETE FROM connector_delivery_receipts
    WHERE connector_instance_id = '00000000-0000-0000-0000-00000000c160'
      AND external_event_id = 'event-1';
  EXCEPTION WHEN raise_exception THEN
    delete_blocked := true;
  END;

  IF NOT conflict_blocked THEN RAISE EXCEPTION 'conflicting duplicate connector event accepted'; END IF;
  IF NOT rewind_blocked THEN RAISE EXCEPTION 'connector stream rewind accepted'; END IF;
  IF NOT delete_blocked THEN RAISE EXCEPTION 'connector delivery evidence deletion accepted'; END IF;
END
$$;

DO $$
BEGIN
  IF (
    SELECT last_applied_sequence
    FROM connector_stream_positions
    WHERE connector_instance_id = '00000000-0000-0000-0000-00000000c160'
      AND stream_id = 'stream-a'
  ) <> 3 THEN
    RAISE EXCEPTION 'connector stream position did not converge to 3';
  END IF;

  IF (
    SELECT count(*)
    FROM connector_delivery_receipts
    WHERE connector_instance_id = '00000000-0000-0000-0000-00000000c160'
      AND state = 'APPLIED'
  ) <> 3 THEN
    RAISE EXCEPTION 'connector applied delivery evidence count mismatch';
  END IF;
END
$$;
