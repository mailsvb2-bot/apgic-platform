BEGIN;

CREATE TABLE identities (
  id uuid PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now(),
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE TABLE identity_roles (
  identity_id uuid NOT NULL REFERENCES identities(id),
  role_code text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (identity_id, role_code)
);

CREATE TABLE organizations (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  status text NOT NULL CHECK (status IN ('ACTIVE', 'ARCHIVED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_memberships (
  organization_id uuid NOT NULL REFERENCES organizations(id),
  identity_id uuid NOT NULL REFERENCES identities(id),
  status text NOT NULL CHECK (status IN ('INVITED','ACTIVE','SUSPENDED','LEFT','REMOVED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, identity_id)
);

CREATE TABLE organization_directions (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL,
  status text NOT NULL CHECK (status IN ('ACTIVE','ARCHIVED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);

CREATE INDEX organization_directions_org_idx ON organization_directions (organization_id, status);

CREATE TABLE products (
  id uuid PRIMARY KEY,
  owner_type text NOT NULL CHECK (owner_type IN ('IDENTITY','ORGANIZATION')),
  owner_id uuid NOT NULL,
  commercial_owner_ref text NOT NULL,
  revenue_beneficiary_ref text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE connector_instances (
  id uuid PRIMARY KEY,
  capability_class text NOT NULL,
  provider_kind text NOT NULL,
  status text NOT NULL CHECK (status IN ('CONFIGURING','ACTIVE','DEGRADED','DISABLED')),
  config_ref text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (capability_class, provider_kind, config_ref)
);

CREATE TABLE outbox_events (
  event_id uuid PRIMARY KEY,
  event_type text NOT NULL,
  schema_version text NOT NULL,
  aggregate_ref text NOT NULL,
  aggregate_version bigint,
  tenant_scope text,
  correlation_id text NOT NULL,
  causation_id text,
  occurred_at timestamptz NOT NULL,
  produced_at timestamptz NOT NULL DEFAULT now(),
  producer text NOT NULL,
  payload jsonb NOT NULL,
  delivery_status text NOT NULL DEFAULT 'PENDING' CHECK (delivery_status IN ('PENDING','DELIVERED')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  delivered_at timestamptz
);

CREATE INDEX outbox_pending_idx ON outbox_events (delivery_status, produced_at)
  WHERE delivery_status = 'PENDING';

CREATE TABLE audit_records (
  id uuid PRIMARY KEY,
  actor_id text NOT NULL,
  action text NOT NULL,
  scope text NOT NULL,
  resource_ref text,
  old_state jsonb,
  new_state jsonb,
  reason text NOT NULL,
  policy_version text NOT NULL,
  correlation_id text,
  occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
  id uuid PRIMARY KEY,
  debit_account_ref text NOT NULL,
  credit_account_ref text NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency char(3) NOT NULL CHECK (currency = upper(currency)),
  provider_evidence_ref text NOT NULL,
  economic_event_ref text,
  correlation_id text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION apgic_reject_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'append-only APGIC evidence table cannot be updated or deleted';
END;
$$;

CREATE TRIGGER audit_records_append_only
BEFORE UPDATE OR DELETE ON audit_records
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE TRIGGER ledger_entries_append_only
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

COMMIT;
