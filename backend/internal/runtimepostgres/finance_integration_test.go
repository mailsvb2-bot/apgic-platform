package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
	"time"
)

const (
	financeTestOrderID      = "00000000-0000-0000-0000-0000000f4450"
	financeTestClientID     = "00000000-0000-0000-0000-00000000b002"
	financeTestSpecialistID = "00000000-0000-0000-0000-00000000b001"
	financeTestProviderB    = "00000000-0000-0000-0000-00000000d002"
	financeTestProviderA    = "00000000-0000-0000-0000-00000000d001"
)

const financeTestCommissionRule = `
INSERT INTO finance_commission_rules (id, policy_version, demand_source, product_ref, jurisdiction,
  currency, basis_points, rounding_mode, platform_fee_recipient_ref, created_at)
VALUES ('00000000-0000-0000-0000-0000000f4601','commission-r2-ci-v1','MARKETPLACE_ORGANIC',
        'consultation_booking','RU','RUB',1000,'FLOOR_MINOR','platform/apgic-fee', now())`

const financeTestProviderProfile = `
INSERT INTO finance_provider_execution_profiles (id, provider_config_id, execution_owner, capabilities,
  certification_evidence_refs, effective_from, created_at)
VALUES ('00000000-0000-0000-0000-0000000f4602','00000000-0000-0000-0000-00000000d102','EXTERNAL_PROVIDER',
        ARRAY['MARKETPLACE_SPLIT','SETTLEMENT_EXECUTION','PAYOUT_EXECUTION'],
        ARRAY['evidence/sandbox-b-split-payout'], now() - interval '1 day', now())`

// Revenue convergence scenario (APGIC-REVENUE-001): Provider payment evidence
// -> APGIC ledger -> completed fulfillment -> commission/allocation ->
// provider settlement/payout evidence -> reconciliation, plus fail-closed
// holds, idempotent provider intake, blocked payout, typed mismatch and an
// append-only chargeback. Runs inside one transaction that is rolled back, on
// top of the CI fixture chain (all ci_*.sql invariants applied first).
func TestR4RevenueConvergenceIsNonCustodialIdempotentAndReconciled(t *testing.T) {
	databaseURL := os.Getenv("APGIC_FINANCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated finance integration database is not configured")
	}
	ctx := context.Background()
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "migrations", "ci_r4_finance_fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	mustExec(t, tx, string(fixture))
	now := time.Now().UTC()

	// 1. Fail closed: no commission rule, then no provider execution profile.
	withSavepoint(t, tx, func() {
		outcome, err := settleOrderTx(ctx, tx, financeTestOrderID, now)
		if err != nil || outcome.Status != "HELD" || outcome.ReasonCode != "POLICY_NOT_CONFIGURED" {
			t.Fatalf("missing policy must hold settlement: %+v %v", outcome, err)
		}
	})
	mustExec(t, tx, financeTestCommissionRule)
	withSavepoint(t, tx, func() {
		outcome, err := settleOrderTx(ctx, tx, financeTestOrderID, now)
		if err != nil || outcome.Status != "HELD" || outcome.ReasonCode != "PROVIDER_PROFILE_MISSING" {
			t.Fatalf("missing provider profile must hold settlement: %+v %v", outcome, err)
		}
	})
	mustExec(t, tx, financeTestProviderProfile)

	// 2. Settlement instruction exactly once, on the original provider.
	outcome, err := settleOrderTx(ctx, tx, financeTestOrderID, now)
	if err != nil || outcome.Status != "INSTRUCTED" {
		t.Fatalf("settlement not instructed: %+v %v", outcome, err)
	}
	instructionID := outcome.SettlementInstructionID
	replay, err := settleOrderTx(ctx, tx, financeTestOrderID, now)
	if err != nil || replay.Status != "ALREADY_INSTRUCTED" || replay.SettlementInstructionID != instructionID {
		t.Fatalf("settlement replay not idempotent: %+v %v", replay, err)
	}
	var fee, beneficiary int64
	if err := tx.QueryRowContext(ctx, `SELECT platform_fee_minor, beneficiary_amount_minor FROM finance_settlement_instructions WHERE id=$1::uuid`, instructionID).Scan(&fee, &beneficiary); err != nil {
		t.Fatal(err)
	}
	if fee != 2000 || beneficiary != 18000 {
		t.Fatalf("allocation does not converge with order snapshot: fee=%d beneficiary=%d", fee, beneficiary)
	}
	assertOutbox(t, tx, "finance-settlement-requested:"+instructionID)

	ev := func(kind string, gross, feeMinor int64) FinanceEvidencePayload {
		return FinanceEvidencePayload{EvidenceKind: kind, SettlementInstructionID: instructionID,
			ProviderReference: "ci-b/" + strings.ToLower(kind), GrossAmountMinor: gross,
			ProviderFeeMinor: feeMinor, Currency: "RUB"}
	}
	record := func(instance, eventID, digest string, payload FinanceEvidencePayload) (FinanceEvidenceDecision, error) {
		return recordFinanceEvidenceTx(ctx, tx, instance, eventID, digest, payload, time.Now().UTC(), time.Now().UTC())
	}
	digest := func(seed string) string { return "sha256:" + strings.Repeat(seed, 64)[:64] }

	// 3. Cross-provider evidence is rejected (no APGIC money bridge).
	withSavepointExpectError(t, tx, func() error {
		_, err := record(financeTestProviderA, "a-settled", digest("a"), ev("SETTLEMENT_EXECUTED", 20000, 300))
		return err
	})

	// 4. Provider stream sequencing is durable: exact replay is idempotent,
	// gaps and stale events are rejected before any financial fact is written.
	streamPayload := ev("PAYOUT_FAILED", 0, 0)
	if d, err := recordSequencedFinanceEvidenceTx(ctx, tx, financeTestProviderB, "b-stream-1", "ci-stream", 1, digest("a"), streamPayload, now, now); err != nil || !d.Accepted {
		t.Fatalf("sequence 1 not accepted: %+v %v", d, err)
	}
	if d, err := recordSequencedFinanceEvidenceTx(ctx, tx, financeTestProviderB, "b-stream-1", "ci-stream", 1, digest("a"), streamPayload, now, now); err != nil || d.Accepted || d.ReasonCode != "FINANCE_EVIDENCE_DUPLICATE" {
		t.Fatalf("same sequenced event not idempotent: %+v %v", d, err)
	}
	if _, err := recordSequencedFinanceEvidenceTx(ctx, tx, financeTestProviderB, "b-stream-3", "ci-stream", 3, digest("c"), streamPayload, now, now); !errors.Is(err, finance.ErrProviderEventSequenceGap) {
		t.Fatalf("sequence gap must be deferred: %v", err)
	}
	if d, err := recordSequencedFinanceEvidenceTx(ctx, tx, financeTestProviderB, "b-stream-2", "ci-stream", 2, digest("b"), streamPayload, now, now); err != nil || !d.Accepted {
		t.Fatalf("sequence 2 not accepted after gap: %+v %v", d, err)
	}
	if _, err := recordSequencedFinanceEvidenceTx(ctx, tx, financeTestProviderB, "b-stream-stale", "ci-stream", 1, digest("d"), streamPayload, now, now); !errors.Is(err, finance.ErrProviderEventSequenceStale) {
		t.Fatalf("stale sequence must be rejected: %v", err)
	}

	// 5. Idempotent provider intake.
	if d, err := record(financeTestProviderB, "b-settled-1", digest("1"), ev("SETTLEMENT_EXECUTED", 20000, 300)); err != nil || !d.Accepted {
		t.Fatalf("settlement evidence not accepted: %+v %v", d, err)
	}
	if d, err := record(financeTestProviderB, "b-settled-1", digest("1"), ev("SETTLEMENT_EXECUTED", 20000, 300)); err != nil || d.Accepted || d.ReasonCode != "FINANCE_EVIDENCE_DUPLICATE" {
		t.Fatalf("duplicate evidence created a second effect: %+v %v", d, err)
	}
	if _, err := record(financeTestProviderB, "b-settled-1", digest("2"), ev("SETTLEMENT_EXECUTED", 20000, 999)); !errors.Is(err, ErrFinanceEvidenceConflict) {
		t.Fatalf("conflicting replay must be rejected: %v", err)
	}

	// 5. Payout blocked without provider eligibility, then instructed once.
	notEligible, eligible := false, true
	blocked := ev("PAYOUT_ELIGIBILITY", 0, 0)
	blocked.PayoutEligible = &notEligible
	if d, err := record(financeTestProviderB, "b-elig-1", digest("3"), blocked); err != nil || d.ReasonCode != "PAYOUT_PROVIDER_ELIGIBILITY_NOT_CONFIRMED" {
		t.Fatalf("payout must stay blocked: %+v %v", d, err)
	}
	allowed := ev("PAYOUT_ELIGIBILITY", 0, 0)
	allowed.PayoutEligible, allowed.ComplianceEvidenceRef = &eligible, "provider-kyc/ci-b/specialist-r4"
	if d, err := record(financeTestProviderB, "b-elig-2", digest("4"), allowed); err != nil || d.ReasonCode != "PAYOUT_INSTRUCTION_READY" {
		t.Fatalf("payout not instructed: %+v %v", d, err)
	}
	if d, err := record(financeTestProviderB, "b-elig-3", digest("5"), allowed); err != nil || d.ReasonCode != "PAYOUT_ALREADY_INSTRUCTED" {
		t.Fatalf("second payout instruction issued: %+v %v", d, err)
	}
	assertOutbox(t, tx, "finance-payout-requested:"+instructionID)
	if _, err := record(financeTestProviderB, "b-paid-1", digest("6"), ev("PAYOUT_EXECUTED", 18000, 0)); err != nil {
		t.Fatal(err)
	}

	// 6. Reconciliation matches and is replay-safe.
	if _, err := record(financeTestProviderB, "b-statement-1", digest("7"), ev("STATEMENT_LINE", 20000, 300)); err != nil {
		t.Fatal(err)
	}
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	report, err := reconcileProviderTx(ctx, tx, financeTestProviderB, start, end, now)
	if err != nil || report.Matched != 1 || report.Mismatched != 0 {
		t.Fatalf("reconciliation did not match: %+v %v", report, err)
	}
	again, err := reconcileProviderTx(ctx, tx, financeTestProviderB, start, end, now)
	if err != nil || again.RunID != report.RunID || again.Matched != 1 {
		t.Fatalf("reconciliation replay rewrote history: %+v %v", again, err)
	}
	status, err := readFinanceOrderStatusTx(ctx, tx, financeTestOrderID, financeTestClientID)
	if err != nil || status == nil || status.RevenueAssurance != "PASS" || status.PayoutStatus != "PROVIDER_PAID_OUT" {
		t.Fatalf("revenue chain did not converge: %+v %v", status, err)
	}
	if s, err := readFinanceOrderStatusTx(ctx, tx, financeTestOrderID, financeTestSpecialistID); err != nil || s == nil {
		t.Fatalf("specialist cannot read own payout status: %v", err)
	}
	if s, err := readFinanceOrderStatusTx(ctx, tx, financeTestOrderID, "00000000-0000-0000-0000-00000000ffff"); err != nil || s != nil {
		t.Fatalf("finance status leaked to unrelated identity: %+v %v", s, err)
	}

	// 7. Provider statement mismatch becomes a typed discrepancy, not drift.
	time.Sleep(2 * time.Millisecond)
	if _, err := record(financeTestProviderB, "b-statement-2", digest("8"), ev("STATEMENT_LINE", 20000, 350)); err != nil {
		t.Fatal(err)
	}
	mismatch, err := reconcileProviderTx(ctx, tx, financeTestProviderB, start, end.Add(time.Minute), now.Add(time.Minute))
	if err != nil || mismatch.Mismatched != 1 {
		t.Fatalf("mismatch not typed: %+v %v", mismatch, err)
	}
	if status, err := readFinanceOrderStatusTx(ctx, tx, financeTestOrderID, financeTestClientID); err != nil || status.RevenueAssurance != "PASS_WITH_EXCEPTION" {
		t.Fatalf("typed mismatch not surfaced: %+v %v", status, err)
	}

	// 7b. Discrepancy resolution is typed, operator-granted and append-only.
	var discrepancyID string
	if err := tx.QueryRowContext(ctx, `
SELECT d.id::text FROM finance_reconciliation_discrepancies d
  JOIN finance_reconciliation_results r ON r.id = d.result_id
 WHERE r.settlement_instruction_id = $1::uuid`, instructionID).Scan(&discrepancyID); err != nil {
		t.Fatal(err)
	}
	if open, err := listOpenFinanceDiscrepanciesTx(ctx, tx, 10); err != nil || len(open) != 1 || open[0].DiscrepancyID != discrepancyID || open[0].ProviderFeeMinor != 350 {
		t.Fatalf("open discrepancy queue wrong: %+v %v", open, err)
	}
	const operatorID = "00000000-0000-0000-0000-0000000f4800"
	resolveAt := now.Add(3 * time.Minute)
	request := func(key, kind string) FinanceDiscrepancyResolutionRequest {
		return FinanceDiscrepancyResolutionRequest{DiscrepancyID: discrepancyID, IdempotencyKey: key, Kind: kind,
			Reason: "provider re-issued statement line", EvidenceRefs: []string{"ticket/fin-ops-1"},
			OperatorID: operatorID, StepUpSessionRef: "session:ci-ops", AuditRef: "audit/ci-ops-1", ResolvedAt: resolveAt}
	}
	mustExec(t, tx, `INSERT INTO identities (id) VALUES ('`+operatorID+`')`)
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, request("k-0", "OPERATIONAL_EXCEPTION_ACCEPTED")); !errors.Is(err, ErrFinanceOperatorNotGranted) {
		t.Fatalf("ungranted operator resolved discrepancy: %v", err)
	}
	mustExec(t, tx, `INSERT INTO finance_operator_grants VALUES ('00000000-0000-0000-0000-0000000f4801','`+operatorID+`',
  'FINANCE_DISCREPANCY_RESOLVE','ci-ops','ci://grant', now() - interval '1 day')`)
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, request("k-1", "WRITE_OFF")); !errors.Is(err, ErrFinanceResolutionInvalid) {
		t.Fatalf("untyped resolution accepted: %v", err)
	}
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, request("k-1", "PROVIDER_CORRECTION_CONFIRMED")); !errors.Is(err, ErrFinanceResolutionInvalid) {
		t.Fatalf("provider correction without evidence accepted: %v", err)
	}
	withSavepoint(t, tx, func() {
		mustExec(t, tx, `INSERT INTO finance_operator_grant_revocations VALUES ('00000000-0000-0000-0000-0000000f4802',
  '00000000-0000-0000-0000-0000000f4801','ci-ops','rotation', now())`)
		if _, err := resolveFinanceDiscrepancyTx(ctx, tx, request("k-r", "OPERATIONAL_EXCEPTION_ACCEPTED")); !errors.Is(err, ErrFinanceOperatorNotGranted) {
			t.Fatalf("revoked operator resolved discrepancy: %v", err)
		}
	})
	correction, err := record(financeTestProviderB, "b-statement-3", digest("c"), ev("STATEMENT_LINE", 20000, 300))
	if err != nil {
		t.Fatal(err)
	}
	stale := request("k-2", "PROVIDER_CORRECTION_CONFIRMED")
	var mismatchStatementID string
	if err := tx.QueryRowContext(ctx, `SELECT r.statement_evidence_id::text FROM finance_reconciliation_results r
  JOIN finance_reconciliation_discrepancies d ON d.result_id = r.id WHERE d.id=$1::uuid`, discrepancyID).Scan(&mismatchStatementID); err != nil {
		t.Fatal(err)
	}
	stale.ProviderEvidenceID = mismatchStatementID
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, stale); !errors.Is(err, ErrFinanceResolutionInvalid) {
		t.Fatalf("correction reusing mismatching statement accepted: %v", err)
	}
	withSavepoint(t, tx, func() {
		rerun, err := reconcileProviderTx(ctx, tx, financeTestProviderB, start, end.Add(2*time.Minute), now.Add(2*time.Minute))
		if err != nil || rerun.Matched != 1 {
			t.Fatalf("corrected statement did not reconcile: %+v %v", rerun, err)
		}
		var matchedID string
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM finance_reconciliation_results WHERE run_id=$1::uuid`, rerun.RunID).Scan(&matchedID); err != nil {
			t.Fatal(err)
		}
		superseded := request("k-s", "SUPERSEDED_BY_MATCHED_RECONCILIATION")
		superseded.SupersedingResultID = matchedID
		if r, err := resolveFinanceDiscrepancyTx(ctx, tx, superseded); err != nil || r.Kind != "SUPERSEDED_BY_MATCHED_RECONCILIATION" {
			t.Fatalf("supersession not recorded: %+v %v", r, err)
		}
	})
	fixed := request("k-3", "PROVIDER_CORRECTION_CONFIRMED")
	fixed.ProviderEvidenceID = correction.EvidenceID
	resolution, err := resolveFinanceDiscrepancyTx(ctx, tx, fixed)
	if err != nil || resolution.Replayed || resolution.ResolvedBy != operatorID {
		t.Fatalf("provider correction not recorded: %+v %v", resolution, err)
	}
	if again, err := resolveFinanceDiscrepancyTx(ctx, tx, fixed); err != nil || !again.Replayed || again.ResolutionID != resolution.ResolutionID {
		t.Fatalf("resolution replay not idempotent: %+v %v", again, err)
	}
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, request("k-4", "OPERATIONAL_EXCEPTION_ACCEPTED")); !errors.Is(err, ErrFinanceDiscrepancyAlreadyResolved) {
		t.Fatalf("second resolution accepted: %v", err)
	}
	conflict := fixed
	conflict.Kind, conflict.ProviderEvidenceID = "OPERATIONAL_EXCEPTION_ACCEPTED", ""
	if _, err := resolveFinanceDiscrepancyTx(ctx, tx, conflict); !errors.Is(err, ErrFinanceResolutionConflict) {
		t.Fatalf("idempotency key reuse with new content accepted: %v", err)
	}
	withSavepointExpectError(t, tx, func() error {
		_, err := tx.ExecContext(ctx, `UPDATE finance_discrepancy_resolutions SET kind='OPERATIONAL_EXCEPTION_ACCEPTED'`)
		return err
	})
	assertOutbox(t, tx, "finance-discrepancy-resolved:"+discrepancyID)
	if status, err := readFinanceOrderStatusTx(ctx, tx, financeTestOrderID, financeTestClientID); err != nil ||
		status.DiscrepancyStatus != "RESOLVED" || status.DiscrepancyResolution != "PROVIDER_CORRECTION_CONFIRMED" {
		t.Fatalf("resolution not surfaced in status: %+v %v", status, err)
	}
	if open, err := listOpenFinanceDiscrepanciesTx(ctx, tx, 10); err != nil || len(open) != 0 {
		t.Fatalf("resolved discrepancy still queued: %+v %v", open, err)
	}

	// 8. Chargeback is a linked, append-only effect bounded by the settled amount.
	if d, err := record(financeTestProviderB, "b-chargeback-1", digest("9"), ev("DISPUTE_CHARGEBACK", 5000, 0)); err != nil || d.ReasonCode != "FINANCE_DISPUTE_RECORDED" {
		t.Fatalf("chargeback not recorded: %+v %v", d, err)
	}
	withSavepointExpectError(t, tx, func() error {
		_, err := record(financeTestProviderB, "b-chargeback-2", digest("b"), ev("DISPUTE_CHARGEBACK", 20000, 0))
		return err
	})
	withSavepointExpectError(t, tx, func() error {
		_, err := tx.ExecContext(ctx, `UPDATE finance_settlement_instructions SET platform_fee_minor = 0 WHERE id=$1::uuid`, instructionID)
		return err
	})
	var originalCapture int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payment_effects WHERE attempt_id='00000000-0000-0000-0000-0000000f4470' AND effect_kind='CAPTURED'`).Scan(&originalCapture); err != nil || originalCapture != 1 {
		t.Fatalf("dispute rewrote original payment facts: %d %v", originalCapture, err)
	}

	// 9. Non-custodial schema: no balance/wallet/custody columns in finance tables.
	var custodial int
	if err := tx.QueryRowContext(ctx, `
SELECT count(*) FROM information_schema.columns
 WHERE table_name LIKE 'finance\_%' AND column_name ~ '(balance|wallet|custody|escrow)'`).Scan(&custodial); err != nil || custodial != 0 {
		t.Fatalf("finance schema contains custodial columns: %d %v", custodial, err)
	}
}

func mustExec(t *testing.T, tx *sql.Tx, statement string) {
	t.Helper()
	if _, err := tx.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func withSavepoint(t *testing.T, tx *sql.Tx, body func()) {
	t.Helper()
	mustExec(t, tx, "SAVEPOINT finance_probe")
	body()
	mustExec(t, tx, "ROLLBACK TO SAVEPOINT finance_probe")
}

func withSavepointExpectError(t *testing.T, tx *sql.Tx, body func() error) {
	t.Helper()
	mustExec(t, tx, "SAVEPOINT finance_probe")
	if err := body(); err == nil {
		t.Fatal("finance invariant violation was accepted")
	}
	mustExec(t, tx, "ROLLBACK TO SAVEPOINT finance_probe")
}

func assertOutbox(t *testing.T, tx *sql.Tx, key string) {
	t.Helper()
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM outbox_events WHERE idempotency_key=$1 AND delivery_status='PENDING'`, key).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox event %s missing: %d %v", key, count, err)
	}
}
