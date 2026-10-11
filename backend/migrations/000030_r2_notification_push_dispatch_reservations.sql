-- One durable, non-replayable provider-dispatch reservation per canonical delivery.
-- This is a dispatch claim, NOT proof that a provider accepted or delivered a push.
-- Uncertain outcomes require reconciliation; automatic timeout replay is forbidden.
CREATE TABLE notification_push_dispatches (
  delivery_id uuid PRIMARY KEY REFERENCES notification_deliveries(id),
  reserved_at timestamptz NOT NULL
);

CREATE TRIGGER notification_push_dispatches_no_update
BEFORE UPDATE ON notification_push_dispatches
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TRIGGER notification_push_dispatches_no_delete
BEFORE DELETE ON notification_push_dispatches
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
