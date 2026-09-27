BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_economic_event_ref_unique
ON ledger_entries (economic_event_ref)
WHERE economic_event_ref IS NOT NULL;

COMMIT;
