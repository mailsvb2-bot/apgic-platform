-- Durable, idempotent receipt for the booking ledger event consumer.
-- This is NOT payment, settlement or payout evidence.
CREATE TABLE IF NOT EXISTS event_gateway_receipts (
  event_id uuid PRIMARY KEY REFERENCES outbox_events(event_id) ON DELETE RESTRICT,
  idempotency_key text NOT NULL UNIQUE,
  event_type text NOT NULL CHECK (event_type = 'booking.ledger_committed'),
  aggregate_ref text NOT NULL,
  payload_digest text NOT NULL CHECK (length(payload_digest) = 64),
  consumed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS event_gateway_receipts_consumed_at_idx ON event_gateway_receipts(consumed_at);
