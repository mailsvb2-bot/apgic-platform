-- R8 Academy: the canonical media-assignment owner already emits
-- academy.media.assigned to outbox_events. The gateway receipt check introduced
-- by 000072 omitted that one event and rejected delivery with SQLSTATE 23514.
-- Keep exactly the prior accepted kinds plus this canonical event. Preserve
-- all existing digest, foreign-key, idempotency and delivery constraints.
ALTER TABLE event_gateway_receipts
  DROP CONSTRAINT event_gateway_receipts_event_type_check;

ALTER TABLE event_gateway_receipts
  ADD CONSTRAINT event_gateway_receipts_event_type_check CHECK (
    event_type = 'booking.ledger_committed'
    OR event_type IN (
      'academy.course.created',
      'academy.version.drafted',
      'academy.module.added',
      'academy.lesson.added',
      'academy.media.assigned',
      'academy.original_rights.attested',
      'academy.version.published',
      'academy.enrollment.created',
      'academy.lesson.completed',
      'academy.learning.completed',
      'academy.assessment.submitted',
      'academy.assessment.evaluated'
    )
  );
