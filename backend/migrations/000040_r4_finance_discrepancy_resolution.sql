-- R4 reconciliation discrepancy resolution (APGIC-RECON-001, APGIC-REVENUE-001).
--
-- A discrepancy is never mutated. Resolution is an append-only fact with a
-- typed kind, the resolving finance operator, step-up session evidence and
-- the authorization audit record. Operator capability is an explicit,
-- append-only grant provisioned out-of-band (no self-service API); revocation
-- is a separate append-only fact.

CREATE TABLE finance_operator_grants (
  id uuid PRIMARY KEY,
  identity_id uuid NOT NULL REFERENCES identities(id),
  capability text NOT NULL CHECK (capability = 'FINANCE_DISCREPANCY_RESOLVE'),
  granted_by text NOT NULL CHECK (btrim(granted_by) <> ''),
  grant_ref text NOT NULL CHECK (btrim(grant_ref) <> ''),
  granted_at timestamptz NOT NULL,
  UNIQUE (identity_id, capability, granted_at)
);

CREATE TABLE finance_operator_grant_revocations (
  id uuid PRIMARY KEY,
  grant_id uuid NOT NULL UNIQUE REFERENCES finance_operator_grants(id),
  revoked_by text NOT NULL CHECK (btrim(revoked_by) <> ''),
  reason text NOT NULL CHECK (btrim(reason) <> ''),
  revoked_at timestamptz NOT NULL
);

CREATE TABLE finance_discrepancy_resolutions (
  id uuid PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  discrepancy_id uuid NOT NULL UNIQUE REFERENCES finance_reconciliation_discrepancies(id),
  kind text NOT NULL CHECK (kind IN (
    'PROVIDER_CORRECTION_CONFIRMED',
    'SUPERSEDED_BY_MATCHED_RECONCILIATION',
    'OPERATIONAL_EXCEPTION_ACCEPTED'
  )),
  provider_evidence_id uuid REFERENCES finance_provider_evidence(id),
  superseding_result_id uuid REFERENCES finance_reconciliation_results(id),
  resolved_by uuid NOT NULL REFERENCES identities(id),
  operator_grant_id uuid NOT NULL REFERENCES finance_operator_grants(id),
  reason text NOT NULL CHECK (length(btrim(reason)) >= 10),
  evidence_refs text[] NOT NULL CHECK (cardinality(evidence_refs) > 0),
  step_up_session_ref text NOT NULL CHECK (btrim(step_up_session_ref) <> ''),
  audit_ref text NOT NULL CHECK (btrim(audit_ref) <> ''),
  resolved_at timestamptz NOT NULL,
  CHECK ((kind = 'PROVIDER_CORRECTION_CONFIRMED') = (provider_evidence_id IS NOT NULL)),
  CHECK ((kind = 'SUPERSEDED_BY_MATCHED_RECONCILIATION') = (superseding_result_id IS NOT NULL))
);

CREATE TRIGGER finance_operator_grants_append_only BEFORE UPDATE OR DELETE ON finance_operator_grants
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_operator_grant_revocations_append_only BEFORE UPDATE OR DELETE ON finance_operator_grant_revocations
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_discrepancy_resolutions_append_only BEFORE UPDATE OR DELETE ON finance_discrepancy_resolutions
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

CREATE OR REPLACE FUNCTION apgic_finance_discrepancy_resolution_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  disc finance_reconciliation_discrepancies%ROWTYPE;
  res finance_reconciliation_results%ROWTYPE;
  run finance_reconciliation_runs%ROWTYPE;
  g finance_operator_grants%ROWTYPE;
  ev finance_provider_evidence%ROWTYPE;
  sup finance_reconciliation_results%ROWTYPE;
BEGIN
  SELECT * INTO disc FROM finance_reconciliation_discrepancies WHERE id = NEW.discrepancy_id;
  SELECT * INTO res FROM finance_reconciliation_results WHERE id = disc.result_id;
  SELECT * INTO run FROM finance_reconciliation_runs WHERE id = res.run_id;
  IF NEW.resolved_at < disc.created_at THEN
    RAISE EXCEPTION 'finance resolution predates discrepancy';
  END IF;

  SELECT * INTO g FROM finance_operator_grants WHERE id = NEW.operator_grant_id;
  IF g.identity_id <> NEW.resolved_by OR g.capability <> 'FINANCE_DISCREPANCY_RESOLVE' OR g.granted_at > NEW.resolved_at THEN
    RAISE EXCEPTION 'finance resolution requires an active operator grant for the resolver';
  END IF;
  IF EXISTS (SELECT 1 FROM finance_operator_grant_revocations r WHERE r.grant_id = g.id AND r.revoked_at <= NEW.resolved_at) THEN
    RAISE EXCEPTION 'finance resolution operator grant is revoked';
  END IF;

  IF NEW.kind = 'PROVIDER_CORRECTION_CONFIRMED' THEN
    SELECT * INTO ev FROM finance_provider_evidence WHERE id = NEW.provider_evidence_id;
    IF ev.provider_instance_id <> run.provider_instance_id
       OR ev.settlement_instruction_id <> res.settlement_instruction_id
       OR ev.evidence_kind <> 'STATEMENT_LINE'
       OR ev.id = res.statement_evidence_id THEN
      RAISE EXCEPTION 'provider correction must be a new STATEMENT_LINE from the same provider for the same settlement';
    END IF;
  ELSIF NEW.kind = 'SUPERSEDED_BY_MATCHED_RECONCILIATION' THEN
    SELECT * INTO sup FROM finance_reconciliation_results WHERE id = NEW.superseding_result_id;
    IF sup.outcome <> 'MATCHED'
       OR sup.settlement_instruction_id <> res.settlement_instruction_id
       OR sup.decided_at <= res.decided_at THEN
      RAISE EXCEPTION 'superseding reconciliation must be a later MATCHED result for the same settlement';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_discrepancy_resolutions_guard BEFORE INSERT ON finance_discrepancy_resolutions
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_discrepancy_resolution_guard();
