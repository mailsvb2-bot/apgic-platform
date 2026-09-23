BEGIN;

CREATE TABLE identities (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE identity_roles (
    identity_id UUID NOT NULL REFERENCES identities(id),
    role_code TEXT NOT NULL CHECK (role_code <> ''),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (identity_id, role_code)
);

CREATE TABLE organizations (
    id UUID PRIMARY KEY,
    legal_name TEXT NOT NULL CHECK (legal_name <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE organization_memberships (
    organization_id UUID NOT NULL REFERENCES organizations(id),
    identity_id UUID NOT NULL REFERENCES identities(id),
    membership_role TEXT NOT NULL CHECK (membership_role <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, identity_id, membership_role)
);

CREATE TABLE organization_directions (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    direction_kind TEXT NOT NULL CHECK (direction_kind <> ''),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type TEXT NOT NULL CHECK (aggregate_type <> ''),
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type <> ''),
    idempotency_key TEXT NOT NULL UNIQUE CHECK (idempotency_key <> ''),
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ,
    delivery_attempts INTEGER NOT NULL DEFAULT 0 CHECK (delivery_attempts >= 0)
);

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    actor_id TEXT NOT NULL CHECK (actor_id <> ''),
    scope TEXT NOT NULL CHECK (scope <> ''),
    action TEXT NOT NULL CHECK (action <> ''),
    reason TEXT NOT NULL CHECK (reason <> ''),
    policy_version TEXT NOT NULL CHECK (policy_version <> ''),
    old_state JSONB,
    new_state JSONB,
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY,
    evidence_id TEXT NOT NULL CHECK (evidence_id <> ''),
    account_code TEXT NOT NULL CHECK (account_code <> ''),
    currency_code TEXT NOT NULL CHECK (currency_code <> ''),
    side TEXT NOT NULL CHECK (side IN ('DEBIT', 'CREDIT')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE payment_routes (
    id UUID PRIMARY KEY,
    provider_code TEXT NOT NULL CHECK (provider_code <> ''),
    method_code TEXT NOT NULL CHECK (method_code <> ''),
    rail_code TEXT NOT NULL CHECK (rail_code <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION apgic_forbid_immutable_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'immutable APGIC evidence cannot be updated or deleted';
END;
$$;

CREATE TRIGGER audit_events_immutable
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_immutable_mutation();

CREATE TRIGGER ledger_entries_immutable
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION apgic_forbid_immutable_mutation();

COMMIT;
