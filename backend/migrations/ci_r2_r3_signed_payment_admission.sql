-- Dedicated R2 -> R3 payment authorization adversarial proof.
-- Run after all numbered migrations. Never synthesize an actual paid booking.
DO $$
DECLARE
  unknown_booking uuid := '00000000-0000-0000-0000-00000000b972';
BEGIN
  IF apgic_live_consultation_payment_authorized(unknown_booking,'ledger/forged',now()) THEN
    RAISE EXCEPTION 'unknown R2 booking granted R3 live room access';
  END IF;
  IF apgic_live_consultation_payment_authorized(NULL,'ledger/forged',now()) THEN
    RAISE EXCEPTION 'null booking granted R3 live room access';
  END IF;
  IF apgic_live_consultation_payment_authorized(unknown_booking,NULL,now()) THEN
    RAISE EXCEPTION 'null entitlement basis granted R3 live room access';
  END IF;
  IF apgic_live_consultation_payment_authorized(unknown_booking,'',now()) THEN
    RAISE EXCEPTION 'empty entitlement basis granted R3 live room access';
  END IF;
  IF apgic_live_consultation_payment_authorized(unknown_booking,'ledger/forged',NULL) THEN
    RAISE EXCEPTION 'missing clock granted R3 live room access';
  END IF;
END $$;

-- No alternate SQL writer may bypass the prepare-time payment check.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger t
      JOIN pg_class c ON c.oid=t.tgrelid
    WHERE c.relname='communication_join_authorizations'
      AND t.tgname='communication_join_authorizations_signed_capture_guard'
      AND t.tgenabled='O'
  ) THEN
    RAISE EXCEPTION 'persisted live room grants lack signed capture guard';
  END IF;
END $$;
