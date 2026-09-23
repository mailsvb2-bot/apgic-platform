BEGIN;

DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
DROP TRIGGER IF EXISTS audit_events_immutable ON audit_events;
DROP FUNCTION IF EXISTS apgic_forbid_immutable_mutation();
DROP TABLE IF EXISTS payment_routes;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS organization_directions;
DROP TABLE IF EXISTS organization_memberships;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS identity_roles;
DROP TABLE IF EXISTS identities;

COMMIT;
