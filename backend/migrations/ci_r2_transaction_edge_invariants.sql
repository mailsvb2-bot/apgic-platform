\set ON_ERROR_STOP on

INSERT INTO connector_instances (
  id, capability_class, provider_kind, status, config_ref
) VALUES
  (
    '00000000-0000-0000-0000-00000000f001',
    'NOTIFICATION_PROVIDER',
    'CI_NOTIFICATION_EDGE',
    'ACTIVE',
    'secret://ci/notification-edge'
  ),
  (
    '00000000-0000-0000-0000-00000000f002',
    'CALENDAR_PROVIDER',
    'CI_CALENDAR_EDGE',
    'ACTIVE',
    'secret://ci/calendar-edge'
  );

DO $$
DECLARE
  created_1 boolean;
  reason_1 text;
  intent_1 uuid;
  created_2 boolean;
  reason_2 text;
  intent_2 uuid;
BEGIN
  SELECT created, reason_code, intent_id
  INTO created_1, reason_1, intent_1
  FROM apgic_create_notification_intent(
    '00000000-0000-0000-0000-00000000f101',
    '00000000-0000-0000-0000-00000000b301',
    'BOOKING_CONFIRMATION',
    'booking-b301:confirmation',
    'SENSITIVE',
    'channel-policy-ci-v1',
    now() + interval '8 minutes'
  );

  SELECT created, reason_code, intent_id
  INTO created_2, reason_2, intent_2
  FROM apgic_create_notification_intent(
    '00000000-0000-0000-0000-00000000f102',
    '00000000-0000-0000-0000-00000000b301',
    'BOOKING_CONFIRMATION',
    'booking-b301:confirmation',
    'SENSITIVE',
    'channel-policy-ci-v1',
    now() + interval '9 minutes'
  );

  IF NOT created_1 OR reason_1 <> 'NOTIF_INTENT_CREATED' OR intent_1 <> '00000000-0000-0000-0000-00000000f101' THEN
    RAISE EXCEPTION 'first NotificationIntent was not created';
  END IF;

  IF created_2 OR reason_2 <> 'NOTIF_INTENT_DUPLICATE' OR intent_2 <> intent_1 THEN
    RAISE EXCEPTION 'duplicate NotificationIntent was not idempotent';
  END IF;

  IF (
    SELECT count(*)
    FROM notification_intents
    WHERE idempotency_key = 'booking-b301:confirmation'
  ) <> 1 THEN
    RAISE EXCEPTION 'duplicate NotificationIntent created multiple business intents';
  END IF;
END
$$;

INSERT INTO notification_deliveries (
  id, intent_id, channel, endpoint_ref, provider_instance_id,
  delivery_idempotency_key, state, created_at, updated_at
) VALUES
  (
    '00000000-0000-0000-0000-00000000f201',
    '00000000-0000-0000-0000-00000000f101',
    'PUSH',
    'device/ci-1',
    '00000000-0000-0000-0000-00000000f001',
    'intent-f101:PUSH:device-ci-1',
    'PENDING',
    now() + interval '10 minutes',
    now() + interval '10 minutes'
  ),
  (
    '00000000-0000-0000-0000-00000000f202',
    '00000000-0000-0000-0000-00000000f101',
    'EMAIL',
    'mail/ci-1',
    '00000000-0000-0000-0000-00000000f001',
    'intent-f101:EMAIL:mail-ci-1',
    'PENDING',
    now() + interval '10 minutes',
    now() + interval '10 minutes'
  );

UPDATE notification_deliveries
SET state = 'SENT',
    provider_reference = 'ci-notification/push-1',
    updated_at = now() + interval '11 minutes'
WHERE id = '00000000-0000-0000-0000-00000000f201';

UPDATE notification_deliveries
SET state = 'DELIVERED',
    updated_at = now() + interval '12 minutes'
WHERE id = '00000000-0000-0000-0000-00000000f201';

DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE notification_deliveries
    SET intent_id = '00000000-0000-0000-0000-00000000f102',
        updated_at = now() + interval '13 minutes'
    WHERE id = '00000000-0000-0000-0000-00000000f202';
  EXCEPTION WHEN foreign_key_violation OR raise_exception THEN
    blocked := true;
  END;

  IF NOT blocked THEN
    RAISE EXCEPTION 'notification transport was allowed to replace canonical business intent';
  END IF;
END
$$;

INSERT INTO calendar_sync_jobs (
  id, booking_id, booking_version, provider_instance_id,
  target_ref, idempotency_key, state, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-00000000f301',
  '00000000-0000-0000-0000-00000000b301',
  1,
  '00000000-0000-0000-0000-00000000f002',
  'calendar/ci-user',
  'booking-b301:v1',
  'PENDING',
  now() + interval '14 minutes',
  now() + interval '14 minutes'
);

UPDATE calendar_sync_jobs
SET state = 'FAILED_RETRYABLE',
    updated_at = now() + interval '15 minutes'
WHERE id = '00000000-0000-0000-0000-00000000f301';

DO $$
BEGIN
  IF (
    SELECT state
    FROM bookings
    WHERE id = '00000000-0000-0000-0000-00000000b301'
  ) <> 'CONFIRMED' THEN
    RAISE EXCEPTION 'calendar provider failure changed canonical Booking truth';
  END IF;
END
$$;

UPDATE calendar_sync_jobs
SET state = 'SYNCED',
    provider_event_ref = 'calendar-event/ci-1',
    updated_at = now() + interval '16 minutes'
WHERE id = '00000000-0000-0000-0000-00000000f301';

INSERT INTO identities (id)
VALUES ('00000000-0000-0000-0000-00000000f401')
ON CONFLICT (id) DO NOTHING;

DO $$
DECLARE
  outcome_1 text;
  mutation_1 uuid;
  state_1 text;
  effect_1 text;
  outcome_2 text;
  mutation_2 uuid;
  state_2 text;
  effect_2 text;
  outcome_3 text;
  mutation_3 uuid;
  state_3 text;
  effect_3 text;
BEGIN
  SELECT outcome, mutation_id, mutation_state, side_effect_ref
  INTO outcome_1, mutation_1, state_1, effect_1
  FROM apgic_claim_client_mutation(
    '00000000-0000-0000-0000-00000000f501',
    '00000000-0000-0000-0000-00000000f401',
    'BOOKING_CONFIRM',
    'offline-mutation-1',
    'sha256:payload-a',
    now() + interval '17 minutes'
  );

  SELECT outcome, mutation_id, mutation_state, side_effect_ref
  INTO outcome_2, mutation_2, state_2, effect_2
  FROM apgic_claim_client_mutation(
    '00000000-0000-0000-0000-00000000f502',
    '00000000-0000-0000-0000-00000000f401',
    'BOOKING_CONFIRM',
    'offline-mutation-1',
    'sha256:payload-a',
    now() + interval '18 minutes'
  );

  SELECT outcome, mutation_id, mutation_state, side_effect_ref
  INTO outcome_3, mutation_3, state_3, effect_3
  FROM apgic_claim_client_mutation(
    '00000000-0000-0000-0000-00000000f503',
    '00000000-0000-0000-0000-00000000f401',
    'BOOKING_CONFIRM',
    'offline-mutation-1',
    'sha256:payload-b',
    now() + interval '19 minutes'
  );

  IF outcome_1 <> 'CLAIMED' OR mutation_1 <> '00000000-0000-0000-0000-00000000f501' THEN
    RAISE EXCEPTION 'first offline mutation was not claimed';
  END IF;
  IF outcome_2 <> 'DUPLICATE' OR mutation_2 <> mutation_1 THEN
    RAISE EXCEPTION 'same offline mutation retry was not idempotent';
  END IF;
  IF outcome_3 <> 'CONFLICT' OR mutation_3 <> mutation_1 THEN
    RAISE EXCEPTION 'changed payload under same idempotency key was not conflict';
  END IF;
END
$$;

SELECT *
FROM apgic_finalize_client_mutation(
  '00000000-0000-0000-0000-00000000f501',
  'booking/00000000-0000-0000-0000-00000000b301',
  now() + interval '20 minutes'
);

DO $$
DECLARE
  retry_outcome text;
  retry_id uuid;
  retry_state text;
  retry_effect text;
BEGIN
  SELECT outcome, mutation_id, mutation_state, side_effect_ref
  INTO retry_outcome, retry_id, retry_state, retry_effect
  FROM apgic_claim_client_mutation(
    '00000000-0000-0000-0000-00000000f504',
    '00000000-0000-0000-0000-00000000f401',
    'BOOKING_CONFIRM',
    'offline-mutation-1',
    'sha256:payload-a',
    now() + interval '21 minutes'
  );

  IF retry_outcome <> 'DUPLICATE_APPLIED' OR
     retry_state <> 'APPLIED' OR
     retry_effect <> 'booking/00000000-0000-0000-0000-00000000b301' THEN
    RAISE EXCEPTION 'client retry after acknowledgement lost canonical side-effect result';
  END IF;
END
$$;
