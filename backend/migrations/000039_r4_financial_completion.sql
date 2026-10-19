-- R4 Financial Completion (APGIC-COMMISSION-001, APGIC-SETTLE-001,
-- APGIC-PAYOUT-001, APGIC-PAY-010/011/018/020, APGIC-DISPUTE-001,
-- APGIC-RECON-001, APGIC-REVENUE-001).
--
-- Non-custodial by construction: APGIC records contractual allocation,
-- provider instructions and provider-reported evidence only. There is no
-- balance, wallet, custody or internal transfer column anywhere below, and the
-- original provider of the captured payment is the only provider that may
-- execute settlement, payout, dispute or statement facts for that order.
-- Every table is append-only; corrections are new linked facts.

CREATE TABLE finance_commission_rules (
  id uuid PRIMARY KEY,
  policy_version text NOT NULL CHECK (btrim(policy_version) <> ''),
  demand_source text NOT NULL CHECK (btrim(demand_source) <> ''),
  product_ref text NOT NULL CHECK (btrim(product_ref) <> ''),
  jurisdiction text NOT NULL CHECK (btrim(jurisdiction) <> ''),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  basis_points bigint NOT NULL CHECK (basis_points BETWEEN 0 AND 10000),
  rounding_mode text NOT NULL CHECK (rounding_mode = 'FLOOR_MINOR'),
  platform_fee_recipient_ref text NOT NULL CHECK (btrim(platform_fee_recipient_ref) <> ''),
  created_at timestamptz NOT NULL,
  UNIQUE (policy_version, demand_source, product_ref, jurisdiction, currency)
);

CREATE TABLE finance_provider_execution_profiles (
  id uuid PRIMARY KEY,
  provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  capabilities text[] NOT NULL CHECK (
    cardinality(capabilities) > 0 AND
    capabilities <@ ARRAY['MARKETPLACE_SPLIT','SETTLEMENT_EXECUTION','PAYOUT_EXECUTION']::text[]
  ),
  certification_evidence_refs text[] NOT NULL CHECK (cardinality(certification_evidence_refs) > 0),
  effective_from timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (provider_config_id, effective_from)
);

CREATE TABLE finance_commission_snapshots (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
  rule_id uuid NOT NULL REFERENCES finance_commission_rules(id),
  policy_version text NOT NULL,
  demand_source text NOT NULL,
  product_ref text NOT NULL,
  jurisdiction text NOT NULL,
  basis_points bigint NOT NULL,
  rounding_mode text NOT NULL,
  basis_amount_minor bigint NOT NULL CHECK (basis_amount_minor > 0),
  commission_minor bigint NOT NULL CHECK (commission_minor >= 0 AND commission_minor <= basis_amount_minor),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  platform_fee_recipient_ref text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE finance_settlement_holds (
  id uuid PRIMARY KEY,
  order_id uuid NOT NULL REFERENCES orders(id),
  reason_code text NOT NULL CHECK (reason_code IN (
    'POLICY_NOT_CONFIGURED',
    'POLICY_SNAPSHOT_MISMATCH',
    'PROVIDER_PROFILE_MISSING',
    'PROVIDER_CAPABILITY_MISSING',
    'PROVIDER_AFFINITY_VIOLATION',
    'REFUNDED_BEFORE_SETTLEMENT'
  )),
  observed_at timestamptz NOT NULL,
  UNIQUE (order_id, reason_code)
);

CREATE TABLE finance_settlement_instructions (
  id uuid PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
  commission_snapshot_id uuid NOT NULL UNIQUE REFERENCES finance_commission_snapshots(id),
  payment_attempt_id uuid NOT NULL REFERENCES payment_attempts(id),
  capture_payment_effect_id uuid NOT NULL UNIQUE REFERENCES payment_effects(id),
  capture_ledger_entry_id uuid NOT NULL REFERENCES ledger_entries(id),
  provider_config_id uuid NOT NULL REFERENCES payment_provider_config_versions(id),
  provider_profile_id uuid NOT NULL REFERENCES finance_provider_execution_profiles(id),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  consultation_session_id uuid NOT NULL UNIQUE REFERENCES consultation_sessions(id),
  completion_evidence_ref text NOT NULL CHECK (btrim(completion_evidence_ref) <> ''),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  total_amount_minor bigint NOT NULL CHECK (total_amount_minor > 0),
  beneficiary_ref text NOT NULL CHECK (btrim(beneficiary_ref) <> ''),
  beneficiary_amount_minor bigint NOT NULL CHECK (beneficiary_amount_minor >= 0),
  platform_fee_recipient_ref text NOT NULL CHECK (btrim(platform_fee_recipient_ref) <> ''),
  platform_fee_minor bigint NOT NULL CHECK (platform_fee_minor >= 0),
  created_at timestamptz NOT NULL,
  CHECK (beneficiary_amount_minor + platform_fee_minor = total_amount_minor)
);

CREATE TABLE finance_provider_evidence (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  external_event_id text NOT NULL CHECK (btrim(external_event_id) <> ''),
  payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^sha256:[0-9a-f]{64}$'),
  evidence_kind text NOT NULL CHECK (evidence_kind IN (
    'SETTLEMENT_EXECUTED',
    'PAYOUT_ELIGIBILITY',
    'PAYOUT_EXECUTED',
    'PAYOUT_FAILED',
    'STATEMENT_LINE',
    'DISPUTE_CHARGEBACK',
    'DISPUTE_REFUND',
    'DISPUTE_REVERSAL'
  )),
  settlement_instruction_id uuid NOT NULL REFERENCES finance_settlement_instructions(id),
  provider_reference text NOT NULL CHECK (btrim(provider_reference) <> ''),
  gross_amount_minor bigint NOT NULL CHECK (gross_amount_minor >= 0),
  provider_fee_minor bigint NOT NULL CHECK (provider_fee_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  payout_eligible boolean,
  compliance_evidence_ref text,
  occurred_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL,
  UNIQUE (provider_instance_id, external_event_id),
  CHECK ((evidence_kind = 'PAYOUT_ELIGIBILITY') = (payout_eligible IS NOT NULL)),
  CHECK (compliance_evidence_ref IS NULL OR btrim(compliance_evidence_ref) <> '')
);

CREATE TABLE finance_payout_decisions (
  id uuid PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  settlement_instruction_id uuid NOT NULL REFERENCES finance_settlement_instructions(id),
  eligibility_evidence_id uuid NOT NULL UNIQUE REFERENCES finance_provider_evidence(id),
  settlement_evidence_id uuid REFERENCES finance_provider_evidence(id),
  decision text NOT NULL CHECK (decision IN ('ALLOW','BLOCKED')),
  reason_code text NOT NULL CHECK (reason_code IN (
    'PAYOUT_INSTRUCTION_READY',
    'PAYOUT_PROVIDER_ELIGIBILITY_NOT_CONFIRMED',
    'PAYOUT_SETTLEMENT_EVIDENCE_MISSING',
    'PAYOUT_PROVIDER_CAPABILITY_MISSING'
  )),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  beneficiary_ref text NOT NULL CHECK (btrim(beneficiary_ref) <> ''),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  decided_at timestamptz NOT NULL,
  CHECK ((decision = 'ALLOW') = (reason_code = 'PAYOUT_INSTRUCTION_READY')),
  CHECK (decision <> 'ALLOW' OR settlement_evidence_id IS NOT NULL)
);

CREATE UNIQUE INDEX finance_payout_one_allow_per_settlement
  ON finance_payout_decisions(settlement_instruction_id)
  WHERE decision = 'ALLOW';

CREATE TABLE finance_reconciliation_runs (
  id uuid PRIMARY KEY,
  provider_instance_id uuid NOT NULL REFERENCES connector_instances(id),
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  started_at timestamptz NOT NULL,
  CHECK (period_end > period_start)
);

CREATE TABLE finance_reconciliation_results (
  id uuid PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES finance_reconciliation_runs(id),
  settlement_instruction_id uuid NOT NULL REFERENCES finance_settlement_instructions(id),
  statement_evidence_id uuid NOT NULL REFERENCES finance_provider_evidence(id),
  settlement_evidence_id uuid NOT NULL REFERENCES finance_provider_evidence(id),
  payout_evidence_id uuid NOT NULL REFERENCES finance_provider_evidence(id),
  outcome text NOT NULL CHECK (outcome IN ('MATCHED','MISMATCH')),
  expected_gross_minor bigint NOT NULL,
  provider_gross_minor bigint NOT NULL,
  expected_fee_minor bigint NOT NULL,
  provider_fee_minor bigint NOT NULL,
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  decided_at timestamptz NOT NULL,
  UNIQUE (run_id, settlement_instruction_id)
);

CREATE TABLE finance_reconciliation_discrepancies (
  id uuid PRIMARY KEY,
  result_id uuid NOT NULL UNIQUE REFERENCES finance_reconciliation_results(id),
  kind text NOT NULL CHECK (kind IN ('PROVIDER_LEDGER_MISMATCH','STATEMENT_MISSING')),
  owner text NOT NULL CHECK (owner = 'FINANCE_OPERATIONS'),
  status text NOT NULL CHECK (status = 'OPEN'),
  evidence_refs text[] NOT NULL CHECK (cardinality(evidence_refs) > 0),
  created_at timestamptz NOT NULL
);

CREATE TABLE finance_dispute_effects (
  id uuid PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE CHECK (btrim(idempotency_key) <> ''),
  settlement_instruction_id uuid NOT NULL REFERENCES finance_settlement_instructions(id),
  provider_evidence_id uuid NOT NULL UNIQUE REFERENCES finance_provider_evidence(id),
  kind text NOT NULL CHECK (kind IN ('CHARGEBACK','REFUND','REVERSAL')),
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  ledger_entry_id uuid NOT NULL UNIQUE REFERENCES ledger_entries(id),
  audit_ref text NOT NULL CHECK (btrim(audit_ref) <> ''),
  execution_owner text NOT NULL CHECK (execution_owner = 'EXTERNAL_PROVIDER'),
  occurred_at timestamptz NOT NULL
);

-- Append-only: every finance fact is immutable history.
CREATE TRIGGER finance_commission_rules_append_only BEFORE UPDATE OR DELETE ON finance_commission_rules
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_provider_execution_profiles_append_only BEFORE UPDATE OR DELETE ON finance_provider_execution_profiles
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_commission_snapshots_append_only BEFORE UPDATE OR DELETE ON finance_commission_snapshots
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_settlement_holds_append_only BEFORE UPDATE OR DELETE ON finance_settlement_holds
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_settlement_instructions_append_only BEFORE UPDATE OR DELETE ON finance_settlement_instructions
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_provider_evidence_append_only BEFORE UPDATE OR DELETE ON finance_provider_evidence
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_payout_decisions_append_only BEFORE UPDATE OR DELETE ON finance_payout_decisions
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_reconciliation_runs_append_only BEFORE UPDATE OR DELETE ON finance_reconciliation_runs
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_reconciliation_results_append_only BEFORE UPDATE OR DELETE ON finance_reconciliation_results
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_reconciliation_discrepancies_append_only BEFORE UPDATE OR DELETE ON finance_reconciliation_discrepancies
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();
CREATE TRIGGER finance_dispute_effects_append_only BEFORE UPDATE OR DELETE ON finance_dispute_effects
  FOR EACH ROW EXECUTE FUNCTION apgic_reject_mutation();

-- Commission snapshot must be derived from an explicit rule and agree with the
-- immutable R2 Order economic snapshot (no hidden default, no recalculation drift).
CREATE OR REPLACE FUNCTION apgic_finance_commission_snapshot_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  rule finance_commission_rules%ROWTYPE;
  ord orders%ROWTYPE;
BEGIN
  SELECT * INTO rule FROM finance_commission_rules WHERE id = NEW.rule_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'commission snapshot requires explicit commission rule';
  END IF;
  SELECT * INTO ord FROM orders WHERE id = NEW.order_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'commission snapshot requires order';
  END IF;
  IF NEW.policy_version <> rule.policy_version OR
     NEW.demand_source <> rule.demand_source OR
     NEW.product_ref <> rule.product_ref OR
     NEW.jurisdiction <> rule.jurisdiction OR
     NEW.currency <> rule.currency OR
     NEW.basis_points <> rule.basis_points OR
     NEW.rounding_mode <> rule.rounding_mode OR
     NEW.platform_fee_recipient_ref <> rule.platform_fee_recipient_ref THEN
    RAISE EXCEPTION 'commission snapshot does not match commission rule';
  END IF;
  IF NEW.basis_amount_minor <> ord.amount_minor OR
     NEW.currency <> ord.currency OR
     NEW.policy_version <> ord.commission_policy_version OR
     NEW.commission_minor <> ord.commission_minor OR
     NEW.commission_minor <> (NEW.basis_amount_minor * NEW.basis_points) / 10000 THEN
    RAISE EXCEPTION 'commission snapshot diverges from immutable order economic snapshot';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_commission_snapshots_guard BEFORE INSERT ON finance_commission_snapshots
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_commission_snapshot_guard();

-- Settlement instruction: completed fulfillment + captured payment + ledger
-- fact + original provider with certified split/settlement capability.
CREATE OR REPLACE FUNCTION apgic_finance_settlement_instruction_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  ord orders%ROWTYPE;
  snap finance_commission_snapshots%ROWTYPE;
  attempt payment_attempts%ROWTYPE;
  effect payment_effects%ROWTYPE;
  entry ledger_entries%ROWTYPE;
  routed_config uuid;
  profile finance_provider_execution_profiles%ROWTYPE;
  session consultation_sessions%ROWTYPE;
BEGIN
  SELECT * INTO ord FROM orders WHERE id = NEW.order_id;
  SELECT * INTO snap FROM finance_commission_snapshots WHERE id = NEW.commission_snapshot_id;
  IF snap.order_id <> NEW.order_id THEN
    RAISE EXCEPTION 'settlement commission snapshot belongs to another order';
  END IF;

  SELECT * INTO attempt FROM payment_attempts WHERE id = NEW.payment_attempt_id;
  IF attempt.order_id <> NEW.order_id OR attempt.state <> 'SUCCEEDED' THEN
    RAISE EXCEPTION 'settlement requires succeeded payment attempt of the same order';
  END IF;
  SELECT * INTO effect FROM payment_effects WHERE id = NEW.capture_payment_effect_id;
  IF effect.attempt_id <> attempt.id OR effect.effect_kind <> 'CAPTURED' OR
     effect.ledger_entry_id IS DISTINCT FROM NEW.capture_ledger_entry_id THEN
    RAISE EXCEPTION 'settlement requires the captured payment effect and its ledger fact';
  END IF;
  SELECT * INTO entry FROM ledger_entries WHERE id = NEW.capture_ledger_entry_id;
  IF entry.amount_minor <> NEW.total_amount_minor OR entry.currency::text <> NEW.currency THEN
    RAISE EXCEPTION 'settlement total diverges from ledger capture fact';
  END IF;

  SELECT provider_config_id INTO routed_config
    FROM payment_routing_decisions WHERE id = attempt.routing_decision_id;
  IF routed_config IS DISTINCT FROM NEW.provider_config_id THEN
    RAISE EXCEPTION 'settlement must use original payment provider';
  END IF;
  SELECT * INTO profile FROM finance_provider_execution_profiles WHERE id = NEW.provider_profile_id;
  IF profile.provider_config_id <> NEW.provider_config_id OR
     NOT (profile.capabilities @> ARRAY['MARKETPLACE_SPLIT','SETTLEMENT_EXECUTION']::text[]) THEN
    RAISE EXCEPTION 'settlement provider lacks certified split/settlement capability';
  END IF;

  SELECT * INTO session FROM consultation_sessions WHERE id = NEW.consultation_session_id;
  IF session.booking_id <> ord.booking_id OR session.state <> 'COMPLETED' THEN
    RAISE EXCEPTION 'settlement requires completed consultation of the order booking';
  END IF;

  IF EXISTS (SELECT 1 FROM refund_requests WHERE order_id = NEW.order_id AND state = 'SUCCEEDED') THEN
    RAISE EXCEPTION 'refunded order cannot be settled';
  END IF;

  IF NEW.total_amount_minor <> ord.amount_minor OR
     NEW.currency <> ord.currency OR
     NEW.platform_fee_minor <> snap.commission_minor OR
     NEW.platform_fee_recipient_ref <> snap.platform_fee_recipient_ref OR
     NEW.beneficiary_ref <> ord.payout_beneficiary_ref THEN
    RAISE EXCEPTION 'settlement allocation diverges from order and commission snapshot';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_settlement_instructions_guard BEFORE INSERT ON finance_settlement_instructions
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_settlement_instruction_guard();

-- Provider evidence can only come from the provider instance that executed
-- the original payment (historical provider affinity; no cross-provider bridge).
CREATE OR REPLACE FUNCTION apgic_finance_provider_evidence_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  instruction finance_settlement_instructions%ROWTYPE;
  instance uuid;
BEGIN
  SELECT * INTO instruction FROM finance_settlement_instructions WHERE id = NEW.settlement_instruction_id;
  SELECT provider_instance_id INTO instance
    FROM payment_provider_config_versions WHERE id = instruction.provider_config_id;
  IF instance IS DISTINCT FROM NEW.provider_instance_id THEN
    RAISE EXCEPTION 'finance evidence must come from original payment provider';
  END IF;
  IF NEW.currency <> instruction.currency THEN
    RAISE EXCEPTION 'finance evidence currency diverges from settlement';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_provider_evidence_guard BEFORE INSERT ON finance_provider_evidence
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_provider_evidence_guard();

CREATE OR REPLACE FUNCTION apgic_finance_payout_decision_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  instruction finance_settlement_instructions%ROWTYPE;
  eligibility finance_provider_evidence%ROWTYPE;
  settled finance_provider_evidence%ROWTYPE;
BEGIN
  SELECT * INTO instruction FROM finance_settlement_instructions WHERE id = NEW.settlement_instruction_id;
  SELECT * INTO eligibility FROM finance_provider_evidence WHERE id = NEW.eligibility_evidence_id;
  IF eligibility.evidence_kind <> 'PAYOUT_ELIGIBILITY' OR
     eligibility.settlement_instruction_id <> NEW.settlement_instruction_id THEN
    RAISE EXCEPTION 'payout decision requires provider eligibility evidence of the same settlement';
  END IF;
  IF NEW.beneficiary_ref <> instruction.beneficiary_ref OR
     NEW.amount_minor <> instruction.beneficiary_amount_minor OR
     NEW.currency <> instruction.currency THEN
    RAISE EXCEPTION 'payout decision diverges from settlement allocation';
  END IF;
  IF NEW.decision = 'ALLOW' THEN
    SELECT * INTO settled FROM finance_provider_evidence WHERE id = NEW.settlement_evidence_id;
    IF settled.evidence_kind <> 'SETTLEMENT_EXECUTED' OR
       settled.settlement_instruction_id <> NEW.settlement_instruction_id THEN
      RAISE EXCEPTION 'payout requires provider settlement evidence';
    END IF;
    IF eligibility.payout_eligible IS NOT TRUE OR eligibility.compliance_evidence_ref IS NULL THEN
      RAISE EXCEPTION 'payout requires provider-reported eligibility and compliance evidence';
    END IF;
    IF NOT EXISTS (
      SELECT 1 FROM finance_provider_execution_profiles
       WHERE id = instruction.provider_profile_id
         AND capabilities @> ARRAY['PAYOUT_EXECUTION']::text[]
    ) THEN
      RAISE EXCEPTION 'payout provider lacks payout execution capability';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_payout_decisions_guard BEFORE INSERT ON finance_payout_decisions
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_payout_decision_guard();

CREATE OR REPLACE FUNCTION apgic_finance_reconciliation_result_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  instruction finance_settlement_instructions%ROWTYPE;
  run finance_reconciliation_runs%ROWTYPE;
  statement finance_provider_evidence%ROWTYPE;
BEGIN
  SELECT * INTO instruction FROM finance_settlement_instructions WHERE id = NEW.settlement_instruction_id;
  SELECT * INTO run FROM finance_reconciliation_runs WHERE id = NEW.run_id;
  SELECT * INTO statement FROM finance_provider_evidence WHERE id = NEW.statement_evidence_id;
  IF statement.evidence_kind <> 'STATEMENT_LINE' OR
     statement.settlement_instruction_id <> NEW.settlement_instruction_id OR
     statement.provider_instance_id <> run.provider_instance_id THEN
    RAISE EXCEPTION 'reconciliation requires provider statement of the same settlement and provider';
  END IF;
  IF NEW.expected_gross_minor <> instruction.total_amount_minor OR
     NEW.provider_gross_minor <> statement.gross_amount_minor OR
     NEW.provider_fee_minor <> statement.provider_fee_minor THEN
    RAISE EXCEPTION 'reconciliation result does not reflect ledger/provider facts';
  END IF;
  IF (NEW.outcome = 'MATCHED') <> (
       NEW.expected_gross_minor = NEW.provider_gross_minor AND
       NEW.expected_fee_minor = NEW.provider_fee_minor AND
       NEW.currency = statement.currency) THEN
    RAISE EXCEPTION 'reconciliation outcome contradicts compared amounts';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_reconciliation_results_guard BEFORE INSERT ON finance_reconciliation_results
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_reconciliation_result_guard();

-- A mismatch must be a typed operational exception, never an untyped gap.
CREATE OR REPLACE FUNCTION apgic_finance_mismatch_requires_discrepancy()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.outcome = 'MISMATCH' AND NOT EXISTS (
    SELECT 1 FROM finance_reconciliation_discrepancies WHERE result_id = NEW.id
  ) THEN
    RAISE EXCEPTION 'reconciliation mismatch requires typed discrepancy';
  END IF;
  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER finance_reconciliation_mismatch_typed
  AFTER INSERT ON finance_reconciliation_results
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_mismatch_requires_discrepancy();

CREATE OR REPLACE FUNCTION apgic_finance_dispute_effect_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  instruction finance_settlement_instructions%ROWTYPE;
  ev finance_provider_evidence%ROWTYPE;
  disputed bigint;
BEGIN
  SELECT * INTO instruction FROM finance_settlement_instructions WHERE id = NEW.settlement_instruction_id FOR UPDATE;
  SELECT * INTO ev FROM finance_provider_evidence WHERE id = NEW.provider_evidence_id;
  IF ev.settlement_instruction_id <> NEW.settlement_instruction_id OR
     ev.evidence_kind <> 'DISPUTE_' || NEW.kind THEN
    RAISE EXCEPTION 'dispute effect requires matching provider dispute evidence';
  END IF;
  IF NEW.currency <> instruction.currency OR NEW.amount_minor <> ev.gross_amount_minor THEN
    RAISE EXCEPTION 'dispute effect diverges from provider evidence';
  END IF;
  SELECT COALESCE(sum(amount_minor), 0) INTO disputed
    FROM finance_dispute_effects WHERE settlement_instruction_id = NEW.settlement_instruction_id;
  IF disputed + NEW.amount_minor > instruction.total_amount_minor THEN
    RAISE EXCEPTION 'dispute effects exceed settled amount';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER finance_dispute_effects_guard BEFORE INSERT ON finance_dispute_effects
  FOR EACH ROW EXECUTE FUNCTION apgic_finance_dispute_effect_guard();
