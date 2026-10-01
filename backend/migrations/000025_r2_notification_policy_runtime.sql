BEGIN;

ALTER TABLE notification_intents
  ADD COLUMN recipient_identity_id uuid REFERENCES identities(id),
  ADD COLUMN template_version text,
  ADD COLUMN channel_preferences jsonb,
  ADD COLUMN not_before timestamptz,
  ADD COLUMN expires_at timestamptz,
  ADD COLUMN consent_basis text,
  ADD COLUMN sensitive_preview_policy text;

ALTER TABLE notification_intents DISABLE TRIGGER notification_intents_append_only;

UPDATE notification_intents i
SET recipient_identity_id = b.client_identity_id,
    template_version = 'transactional-v1',
    channel_preferences = '{"PUSH":true,"EMAIL":true,"SMS":false}'::jsonb,
    not_before = i.created_at,
    consent_basis = 'TRANSACTIONAL_NECESSITY',
    sensitive_preview_policy = 'GENERIC_ONLY'
FROM bookings b
WHERE b.id = i.booking_id;

ALTER TABLE notification_intents ENABLE TRIGGER notification_intents_append_only;

ALTER TABLE notification_intents
  ALTER COLUMN recipient_identity_id SET NOT NULL,
  ALTER COLUMN template_version SET NOT NULL,
  ALTER COLUMN template_version SET DEFAULT 'transactional-v1',
  ALTER COLUMN channel_preferences SET NOT NULL,
  ALTER COLUMN channel_preferences SET DEFAULT '{"PUSH":true,"EMAIL":true,"SMS":false}'::jsonb,
  ALTER COLUMN not_before SET NOT NULL,
  ALTER COLUMN consent_basis SET NOT NULL,
  ALTER COLUMN consent_basis SET DEFAULT 'TRANSACTIONAL_NECESSITY',
  ALTER COLUMN sensitive_preview_policy SET NOT NULL,
  ALTER COLUMN sensitive_preview_policy SET DEFAULT 'GENERIC_ONLY',
  ADD CONSTRAINT notification_intents_template_version_nonempty CHECK (btrim(template_version) <> ''),
  ADD CONSTRAINT notification_intents_channel_preferences_shape CHECK (
    jsonb_typeof(channel_preferences) = 'object' AND
    jsonb_typeof(channel_preferences -> 'PUSH') = 'boolean' AND
    jsonb_typeof(channel_preferences -> 'EMAIL') = 'boolean' AND
    jsonb_typeof(channel_preferences -> 'SMS') = 'boolean'
  ),
  ADD CONSTRAINT notification_intents_consent_basis_nonempty CHECK (btrim(consent_basis) <> ''),
  ADD CONSTRAINT notification_intents_sensitive_preview_policy
    CHECK (sensitive_preview_policy IN ('GENERIC_ONLY','FULL_ALLOWED')),
  ADD CONSTRAINT notification_intents_expiry_after_not_before
    CHECK (expires_at IS NULL OR expires_at > not_before);

CREATE OR REPLACE FUNCTION apgic_notification_intent_policy_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  booking_client uuid;
BEGIN
  SELECT client_identity_id
  INTO booking_client
  FROM bookings
  WHERE id = NEW.booking_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'NotificationIntent requires canonical Booking';
  END IF;

  IF NEW.recipient_identity_id IS NULL THEN
    NEW.recipient_identity_id := booking_client;
  ELSIF NEW.recipient_identity_id <> booking_client THEN
    RAISE EXCEPTION 'NotificationIntent recipient must match canonical Booking client';
  END IF;

  NEW.template_version := COALESCE(NULLIF(btrim(NEW.template_version), ''), 'transactional-v1');
  NEW.channel_preferences := COALESCE(
    NEW.channel_preferences,
    '{"PUSH":true,"EMAIL":true,"SMS":false}'::jsonb
  );
  NEW.not_before := COALESCE(NEW.not_before, NEW.created_at);
  NEW.consent_basis := COALESCE(NULLIF(btrim(NEW.consent_basis), ''), 'TRANSACTIONAL_NECESSITY');
  NEW.sensitive_preview_policy := COALESCE(
    NULLIF(btrim(NEW.sensitive_preview_policy), ''),
    'GENERIC_ONLY'
  );

  IF NEW.transactional AND upper(NEW.consent_basis) = 'MARKETING' THEN
    RAISE EXCEPTION 'transactional NotificationIntent cannot inherit marketing consent';
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER notification_intents_policy_guard
BEFORE INSERT ON notification_intents
FOR EACH ROW EXECUTE FUNCTION apgic_notification_intent_policy_guard();

COMMIT;
