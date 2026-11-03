-- R8: the shared durable receipt gateway now accepts canonical Academy events.
-- Previously, migration 000042 admitted only booking.ledger_committed, so
-- Academy deliveries reached the handler but failed INSERT with SQLSTATE 23514,
-- returning HTTP 503 and leaving the canonical outbox rows pending.
-- Preserve the foreign key, unique idempotency key, digest and consumer
-- verification against the complete persisted outbox event.
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
      'academy.original_rights.attested',
      'academy.version.published',
      'academy.enrollment.created',
      'academy.lesson.completed',
      'academy.learning.completed',
      'academy.assessment.submitted',
      'academy.assessment.evaluated'
    )
  );
