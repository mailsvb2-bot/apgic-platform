\set ON_ERROR_STOP on
-- R4 finance PostgreSQL invariants. Runs after every other ci_*.sql file and
-- leaves no trace: the whole probe is rolled back.
BEGIN;
\ir ci_r4_finance_fixture.sql

INSERT INTO finance_commission_rules (id, policy_version, demand_source, product_ref, jurisdiction,
  currency, basis_points, rounding_mode, platform_fee_recipient_ref, created_at)
VALUES ('00000000-0000-0000-0000-0000000f4601','commission-r2-ci-v1','MARKETPLACE_ORGANIC',
        'consultation_booking','RU','RUB',1000,'FLOOR_MINOR','platform/apgic-fee', now());
INSERT INTO finance_provider_execution_profiles (id, provider_config_id, execution_owner, capabilities,
  certification_evidence_refs, effective_from, created_at)
VALUES
('00000000-0000-0000-0000-0000000f4602','00000000-0000-0000-0000-00000000d102','EXTERNAL_PROVIDER',
 ARRAY['MARKETPLACE_SPLIT','SETTLEMENT_EXECUTION','PAYOUT_EXECUTION'],ARRAY['evidence/sandbox-b-split-payout'],
 now() - interval '1 day', now()),
('00000000-0000-0000-0000-0000000f4603','00000000-0000-0000-0000-00000000d101','EXTERNAL_PROVIDER',
 ARRAY['MARKETPLACE_SPLIT','SETTLEMENT_EXECUTION'],ARRAY['evidence/sandbox-a-split'],
 now() - interval '1 day', now());

DO $$
DECLARE blocked boolean;
BEGIN
  -- Custodial capability names are not representable.
  blocked := false;
  BEGIN
    INSERT INTO finance_provider_execution_profiles VALUES ('00000000-0000-0000-0000-0000000f4699',
      '00000000-0000-0000-0000-00000000d102','APGIC',ARRAY['PAYOUT_EXECUTION'],ARRAY['x'],now(),now());
  EXCEPTION WHEN check_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'APGIC-executed payout profile was accepted'; END IF;

  -- Commission snapshot must equal the immutable order economics.
  blocked := false;
  BEGIN
    INSERT INTO finance_commission_snapshots VALUES ('00000000-0000-0000-0000-0000000f4698',
      '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4601','commission-r2-ci-v1',
      'MARKETPLACE_ORGANIC','consultation_booking','RU',1000,'FLOOR_MINOR',20000,1500,'RUB','platform/apgic-fee',now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'commission drifted from order snapshot'; END IF;
END
$$;

INSERT INTO finance_commission_snapshots VALUES ('00000000-0000-0000-0000-0000000f4610',
  '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4601','commission-r2-ci-v1',
  'MARKETPLACE_ORGANIC','consultation_booking','RU',1000,'FLOOR_MINOR',20000,2000,'RUB','platform/apgic-fee',now());

DO $$
DECLARE blocked boolean;
BEGIN
  -- Settlement on a provider other than the one that captured the payment.
  blocked := false;
  BEGIN
    INSERT INTO finance_settlement_instructions VALUES ('00000000-0000-0000-0000-0000000f4697','settlement:cross',
      '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4610',
      '00000000-0000-0000-0000-0000000f4470','00000000-0000-0000-0000-0000000f4491',
      '00000000-0000-0000-0000-0000000f4490','00000000-0000-0000-0000-00000000d101',
      '00000000-0000-0000-0000-0000000f4603','EXTERNAL_PROVIDER','00000000-0000-0000-0000-0000000f4501',
      'provider-evidence/r4-ended','RUB',20000,'identity/specialist-r4',18000,'platform/apgic-fee',2000,now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'cross-provider settlement accepted'; END IF;

  -- Allocation must split exactly the captured amount.
  blocked := false;
  BEGIN
    INSERT INTO finance_settlement_instructions VALUES ('00000000-0000-0000-0000-0000000f4696','settlement:skim',
      '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4610',
      '00000000-0000-0000-0000-0000000f4470','00000000-0000-0000-0000-0000000f4491',
      '00000000-0000-0000-0000-0000000f4490','00000000-0000-0000-0000-00000000d102',
      '00000000-0000-0000-0000-0000000f4602','EXTERNAL_PROVIDER','00000000-0000-0000-0000-0000000f4501',
      'provider-evidence/r4-ended','RUB',20000,'identity/specialist-r4',17000,'platform/apgic-fee',2000,now());
  EXCEPTION WHEN check_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'non-converging allocation accepted'; END IF;
END
$$;

INSERT INTO finance_settlement_instructions VALUES ('00000000-0000-0000-0000-0000000f4620','settlement:ci-r4',
  '00000000-0000-0000-0000-0000000f4450','00000000-0000-0000-0000-0000000f4610',
  '00000000-0000-0000-0000-0000000f4470','00000000-0000-0000-0000-0000000f4491',
  '00000000-0000-0000-0000-0000000f4490','00000000-0000-0000-0000-00000000d102',
  '00000000-0000-0000-0000-0000000f4602','EXTERNAL_PROVIDER','00000000-0000-0000-0000-0000000f4501',
  'provider-evidence/r4-ended','RUB',20000,'identity/specialist-r4',18000,'platform/apgic-fee',2000,now());

INSERT INTO finance_provider_evidence VALUES
('00000000-0000-0000-0000-0000000f4631','00000000-0000-0000-0000-00000000d002','b-settled',
 'sha256:' || repeat('1',64),'SETTLEMENT_EXECUTED','00000000-0000-0000-0000-0000000f4620','ci-b/settled',
 20000,300,'RUB',NULL,NULL,now(),now()),
('00000000-0000-0000-0000-0000000f4632','00000000-0000-0000-0000-00000000d002','b-elig-no',
 'sha256:' || repeat('2',64),'PAYOUT_ELIGIBILITY','00000000-0000-0000-0000-0000000f4620','ci-b/elig',
 0,0,'RUB',false,NULL,now(),now()),
('00000000-0000-0000-0000-0000000f4633','00000000-0000-0000-0000-00000000d002','b-statement',
 'sha256:' || repeat('3',64),'STATEMENT_LINE','00000000-0000-0000-0000-0000000f4620','ci-b/statement',
 20000,350,'RUB',NULL,NULL,now(),now()),
('00000000-0000-0000-0000-0000000f4634','00000000-0000-0000-0000-00000000d002','b-paid',
 'sha256:' || repeat('4',64),'PAYOUT_EXECUTED','00000000-0000-0000-0000-0000000f4620','ci-b/paid',
 18000,0,'RUB',NULL,NULL,now(),now());

INSERT INTO finance_reconciliation_runs VALUES ('00000000-0000-0000-0000-0000000f4640',
  '00000000-0000-0000-0000-00000000d002', now() - interval '1 hour', now() + interval '1 hour', now());

DO $$
DECLARE blocked boolean;
BEGIN
  blocked := false;
  BEGIN
    UPDATE finance_settlement_instructions SET platform_fee_minor = 0 WHERE id='00000000-0000-0000-0000-0000000f4620';
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'settlement instruction history was mutable'; END IF;

  blocked := false;
  BEGIN
    INSERT INTO finance_provider_evidence VALUES ('00000000-0000-0000-0000-0000000f4695',
      '00000000-0000-0000-0000-00000000d001','a-settled','sha256:' || repeat('5',64),'SETTLEMENT_EXECUTED',
      '00000000-0000-0000-0000-0000000f4620','ci-a/settled',20000,300,'RUB',NULL,NULL,now(),now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'cross-provider finance evidence accepted'; END IF;

  blocked := false;
  BEGIN
    INSERT INTO finance_provider_evidence VALUES ('00000000-0000-0000-0000-0000000f4694',
      '00000000-0000-0000-0000-00000000d002','b-settled','sha256:' || repeat('6',64),'SETTLEMENT_EXECUTED',
      '00000000-0000-0000-0000-0000000f4620','ci-b/settled-again',20000,300,'RUB',NULL,NULL,now(),now());
  EXCEPTION WHEN unique_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'duplicate provider event created a second fact'; END IF;

  blocked := false;
  BEGIN
    INSERT INTO finance_payout_decisions VALUES ('00000000-0000-0000-0000-0000000f4693','payout:forced',
      '00000000-0000-0000-0000-0000000f4620','00000000-0000-0000-0000-0000000f4632',
      '00000000-0000-0000-0000-0000000f4631','ALLOW','PAYOUT_INSTRUCTION_READY','EXTERNAL_PROVIDER',
      'identity/specialist-r4',18000,'RUB',now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'payout allowed without provider eligibility'; END IF;

  blocked := false;
  BEGIN
    SET CONSTRAINTS finance_reconciliation_mismatch_typed IMMEDIATE;
    INSERT INTO finance_reconciliation_results VALUES ('00000000-0000-0000-0000-0000000f4692',
      '00000000-0000-0000-0000-0000000f4640','00000000-0000-0000-0000-0000000f4620',
      '00000000-0000-0000-0000-0000000f4633','00000000-0000-0000-0000-0000000f4631',
      '00000000-0000-0000-0000-0000000f4634','MISMATCH',20000,20000,300,350,'RUB',now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'untyped reconciliation mismatch accepted'; END IF;

  blocked := false;
  BEGIN
    INSERT INTO finance_reconciliation_results VALUES ('00000000-0000-0000-0000-0000000f4691',
      '00000000-0000-0000-0000-0000000f4640','00000000-0000-0000-0000-0000000f4620',
      '00000000-0000-0000-0000-0000000f4633','00000000-0000-0000-0000-0000000f4631',
      '00000000-0000-0000-0000-0000000f4634','MATCHED',20000,20000,300,350,'RUB',now());
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'mismatched amounts recorded as MATCHED'; END IF;
END
$$;

SET CONSTRAINTS finance_reconciliation_mismatch_typed DEFERRED;
INSERT INTO finance_reconciliation_results VALUES ('00000000-0000-0000-0000-0000000f4650',
  '00000000-0000-0000-0000-0000000f4640','00000000-0000-0000-0000-0000000f4620',
  '00000000-0000-0000-0000-0000000f4633','00000000-0000-0000-0000-0000000f4631',
  '00000000-0000-0000-0000-0000000f4634','MISMATCH',20000,20000,300,350,'RUB',now());
INSERT INTO finance_reconciliation_discrepancies VALUES ('00000000-0000-0000-0000-0000000f4651',
  '00000000-0000-0000-0000-0000000f4650','PROVIDER_LEDGER_MISMATCH','FINANCE_OPERATIONS','OPEN',
  ARRAY['finance-evidence/00000000-0000-0000-0000-0000000f4633'],now());
SET CONSTRAINTS finance_reconciliation_mismatch_typed IMMEDIATE;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
     WHERE table_name LIKE 'finance\_%' AND column_name ~ '(balance|wallet|custody|escrow)'
  ) THEN
    RAISE EXCEPTION 'finance schema exposes custodial columns';
  END IF;
END
$$;


-- Discrepancy resolution (000040): typed, append-only, operator-granted.
INSERT INTO identities (id) VALUES ('00000000-0000-0000-0000-0000000f4700'), ('00000000-0000-0000-0000-0000000f4701');
INSERT INTO finance_operator_grants VALUES ('00000000-0000-0000-0000-0000000f4710',
  '00000000-0000-0000-0000-0000000f4700','FINANCE_DISCREPANCY_RESOLVE','ci-ops','ci://grant',now() - interval '1 day');

DO $$
DECLARE blocked boolean;
BEGIN
  -- resolver without grant (grant belongs to another identity)
  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4720','ci-res-x',
      '00000000-0000-0000-0000-0000000f4651','OPERATIONAL_EXCEPTION_ACCEPTED',NULL,NULL,
      '00000000-0000-0000-0000-0000000f4701','00000000-0000-0000-0000-0000000f4710',
      'accepted by finance ops','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'resolution without operator grant accepted'; END IF;

  -- provider correction without provider evidence
  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4721','ci-res-y',
      '00000000-0000-0000-0000-0000000f4651','PROVIDER_CORRECTION_CONFIRMED',NULL,NULL,
      '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
      'provider corrected statement','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');
  EXCEPTION WHEN check_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'untyped provider correction accepted'; END IF;

  -- provider correction reusing the mismatching statement line
  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4722','ci-res-z',
      '00000000-0000-0000-0000-0000000f4651','PROVIDER_CORRECTION_CONFIRMED','00000000-0000-0000-0000-0000000f4633',NULL,
      '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
      'provider corrected statement','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'correction reusing mismatching evidence accepted'; END IF;

  -- superseded by a MISMATCH result (itself)
  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4723','ci-res-w',
      '00000000-0000-0000-0000-0000000f4651','SUPERSEDED_BY_MATCHED_RECONCILIATION',NULL,'00000000-0000-0000-0000-0000000f4650',
      '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
      'superseded by later run','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');
  EXCEPTION WHEN raise_exception THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'supersession by non-MATCHED result accepted'; END IF;

  -- blank reason
  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4724','ci-res-v',
      '00000000-0000-0000-0000-0000000f4651','OPERATIONAL_EXCEPTION_ACCEPTED',NULL,NULL,
      '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
      '  ','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');
  EXCEPTION WHEN check_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'resolution without reason accepted'; END IF;
END
$$;

INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4725','ci-res-ok',
  '00000000-0000-0000-0000-0000000f4651','OPERATIONAL_EXCEPTION_ACCEPTED',NULL,NULL,
  '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
  'accepted by finance ops','{ci://ticket}','session:ci','audit:ci',now() + interval '1 hour');

DO $$
DECLARE blocked boolean;
BEGIN
  blocked := false;
  BEGIN
    UPDATE finance_discrepancy_resolutions SET reason = 'rewritten history' WHERE id = '00000000-0000-0000-0000-0000000f4725';
  EXCEPTION WHEN OTHERS THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'resolution history mutated'; END IF;

  blocked := false;
  BEGIN
    INSERT INTO finance_discrepancy_resolutions VALUES ('00000000-0000-0000-0000-0000000f4726','ci-res-dup',
      '00000000-0000-0000-0000-0000000f4651','OPERATIONAL_EXCEPTION_ACCEPTED',NULL,NULL,
      '00000000-0000-0000-0000-0000000f4700','00000000-0000-0000-0000-0000000f4710',
      'second resolution attempt','{ci://ticket}','session:ci','audit:ci',now() + interval '2 hour');
  EXCEPTION WHEN unique_violation THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'discrepancy resolved twice'; END IF;

  INSERT INTO finance_operator_grant_revocations VALUES ('00000000-0000-0000-0000-0000000f4711',
    '00000000-0000-0000-0000-0000000f4710','ci-ops','rotation',now());
  blocked := false;
  BEGIN
    DELETE FROM finance_operator_grants WHERE id = '00000000-0000-0000-0000-0000000f4710';
  EXCEPTION WHEN OTHERS THEN blocked := true; END;
  IF NOT blocked THEN RAISE EXCEPTION 'operator grant deleted'; END IF;
END
$$;

ROLLBACK;
