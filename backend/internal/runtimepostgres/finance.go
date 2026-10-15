package runtimepostgres

// R4 Financial Completion runtime (non-custodial).
//
// APGIC derives commission/allocation from the immutable Order economic
// snapshot, emits provider-executed settlement/payout instructions through the
// transactional outbox (delivered by the existing outbox worker / connector
// gateway), records signed provider evidence idempotently, reconciles provider
// statements against ledger-backed facts and records disputes as linked,
// append-only effects. No method here holds, moves or nets funds.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const (
	financeTimeout = 5 * time.Second

	// Demand attribution is not yet modelled on Orders (R10 growth lane), so
	// marketplace-intermediary orders are attributed to the organic source.
	// Any other platform role has no configured source and fails closed.
	financeOrganicDemandSource = "MARKETPLACE_ORGANIC"
	financeConsultationProduct = "consultation_booking"
)

var financeUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var (
	ErrFinanceEvidenceInvalid  = errors.New("finance provider evidence is invalid")
	ErrFinanceEvidenceConflict = errors.New("finance provider event id reused with different content")
	ErrFinanceNotFound         = errors.New("finance record not found")
)

type FinanceSettlementOutcome struct {
	OrderID                 string `json:"order_id"`
	Status                  string `json:"status"` // INSTRUCTED, ALREADY_INSTRUCTED, HELD
	ReasonCode              string `json:"reason_code,omitempty"`
	SettlementInstructionID string `json:"settlement_instruction_id,omitempty"`
}

type FinanceEvidencePayload struct {
	EvidenceKind            string `json:"evidence_kind"`
	SettlementInstructionID string `json:"settlement_instruction_id"`
	ProviderReference       string `json:"provider_reference"`
	GrossAmountMinor        int64  `json:"gross_amount_minor"`
	ProviderFeeMinor        int64  `json:"provider_fee_minor"`
	Currency                string `json:"currency"`
	PayoutEligible          *bool  `json:"payout_eligible,omitempty"`
	ComplianceEvidenceRef   string `json:"compliance_evidence_ref,omitempty"`
}

type FinanceEvidenceDecision = finance.EvidenceDecision

type FinanceReconciliationReport struct {
	RunID      string `json:"run_id"`
	Matched    int    `json:"matched"`
	Mismatched int    `json:"mismatched"`
	Pending    int    `json:"pending"`
}

type FinanceOrderStatus = finance.OrderStatus

// ---------------------------------------------------------------- settlement

// SettleCompletedConsultations is the orchestration entrypoint: every Order
// whose consultation is COMPLETED and whose payment is captured with a ledger
// fact, and that has neither an instruction nor a recorded hold, is settled.
func (c *Checker) SettleCompletedConsultations(ctx context.Context, limit int) ([]FinanceSettlementOutcome, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("finance settlement batch must be within 1..1000")
	}
	ids, err := c.financeSettlementCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}
	outcomes := make([]FinanceSettlementOutcome, 0, len(ids))
	for _, orderID := range ids {
		outcome, err := c.SettleOrder(ctx, orderID)
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func (c *Checker) financeSettlementCandidates(ctx context.Context, limit int) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(queryCtx, financeCandidateSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("query finance settlement candidates: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const financeCandidateSQL = `
SELECT o.id::text
  FROM orders o
  JOIN consultation_sessions s ON s.booking_id = o.booking_id AND s.state = 'COMPLETED'
  JOIN payment_attempts a ON a.order_id = o.id AND a.state = 'SUCCEEDED'
  JOIN payment_effects e ON e.attempt_id = a.id AND e.effect_kind = 'CAPTURED'
 WHERE NOT EXISTS (SELECT 1 FROM finance_settlement_instructions i WHERE i.order_id = o.id)
   AND NOT EXISTS (SELECT 1 FROM finance_settlement_holds h WHERE h.order_id = o.id)
 ORDER BY o.captured_at, o.id
 LIMIT $1`

func (c *Checker) SettleOrder(ctx context.Context, orderID string) (FinanceSettlementOutcome, error) {
	txCtx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(txCtx, nil)
	if err != nil {
		return FinanceSettlementOutcome{}, fmt.Errorf("begin finance settlement: %w", err)
	}
	defer tx.Rollback()
	outcome, err := settleOrderTx(txCtx, tx, orderID, time.Now().UTC())
	if err != nil {
		return FinanceSettlementOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return FinanceSettlementOutcome{}, fmt.Errorf("commit finance settlement: %w", err)
	}
	return outcome, nil
}

type financeOrderFacts struct {
	OrderID, BookingID, Currency, CommissionPolicyVersion, PlatformRole, BeneficiaryRef string
	AmountMinor, CommissionMinor                                                        int64
	SessionID, CompletionEvidenceRef                                                    string
	AttemptID, EffectID, LedgerEntryID, CaptureEvidenceRef                              string
	ProviderConfigID, ProviderInstanceID, Jurisdiction                                  string
}

func loadFinanceOrderFactsTx(ctx context.Context, tx *sql.Tx, orderID string) (financeOrderFacts, error) {
	var f financeOrderFacts
	var completion sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT o.id::text, o.booking_id::text, o.currency, o.commission_policy_version, o.platform_role,
       o.payout_beneficiary_ref, o.amount_minor, o.commission_minor,
       s.id::text,
       (SELECT f.evidence_ref FROM consultation_lifecycle_facts f
         WHERE f.session_id = s.id AND f.fact_type = 'ENDED'
         ORDER BY f.occurred_at DESC, f.id DESC LIMIT 1),
       a.id::text, e.id::text, e.ledger_entry_id::text, e.provider_evidence_ref,
       d.provider_config_id::text, p.provider_instance_id::text, d.jurisdiction_code
  FROM orders o
  JOIN consultation_sessions s ON s.booking_id = o.booking_id AND s.state = 'COMPLETED'
  JOIN payment_attempts a ON a.order_id = o.id AND a.state = 'SUCCEEDED'
  JOIN payment_effects e ON e.attempt_id = a.id AND e.effect_kind = 'CAPTURED'
  JOIN payment_routing_decisions d ON d.id = a.routing_decision_id
  JOIN payment_provider_config_versions p ON p.id = d.provider_config_id
 WHERE o.id = $1::uuid`, orderID).Scan(
		&f.OrderID, &f.BookingID, &f.Currency, &f.CommissionPolicyVersion, &f.PlatformRole,
		&f.BeneficiaryRef, &f.AmountMinor, &f.CommissionMinor,
		&f.SessionID, &completion,
		&f.AttemptID, &f.EffectID, &f.LedgerEntryID, &f.CaptureEvidenceRef,
		&f.ProviderConfigID, &f.ProviderInstanceID, &f.Jurisdiction,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrFinanceNotFound
	}
	if err != nil {
		return f, fmt.Errorf("load finance order facts: %w", err)
	}
	f.CompletionEvidenceRef = completion.String
	return f, nil
}

func settleOrderTx(ctx context.Context, tx *sql.Tx, orderID string, now time.Time) (FinanceSettlementOutcome, error) {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('finance-settle:' || $1))`, orderID); err != nil {
		return FinanceSettlementOutcome{}, fmt.Errorf("lock finance settlement: %w", err)
	}
	outcome := FinanceSettlementOutcome{OrderID: orderID}
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT id::text FROM finance_settlement_instructions WHERE order_id = $1::uuid`, orderID).Scan(&existing)
	if err == nil {
		outcome.Status, outcome.SettlementInstructionID = "ALREADY_INSTRUCTED", existing
		return outcome, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return outcome, fmt.Errorf("read settlement instruction: %w", err)
	}

	facts, err := loadFinanceOrderFactsTx(ctx, tx, orderID)
	if err != nil {
		return outcome, err
	}
	hold := func(reason string) (FinanceSettlementOutcome, error) {
		id, err := persistentid.FromRef("finance-settlement-hold", orderID+"/"+reason)
		if err != nil {
			return outcome, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_settlement_holds (id, order_id, reason_code, observed_at)
VALUES ($1::uuid, $2::uuid, $3, $4) ON CONFLICT (order_id, reason_code) DO NOTHING`,
			id, orderID, reason, now); err != nil {
			return outcome, fmt.Errorf("record settlement hold: %w", err)
		}
		outcome.Status, outcome.ReasonCode = "HELD", reason
		return outcome, nil
	}

	var refunded bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM refund_requests WHERE order_id = $1::uuid AND state = 'SUCCEEDED')`, orderID).Scan(&refunded); err != nil {
		return outcome, fmt.Errorf("read refund state: %w", err)
	}
	if refunded {
		return hold("REFUNDED_BEFORE_SETTLEMENT")
	}

	demandSource := ""
	if facts.PlatformRole == "MARKETPLACE_INTERMEDIARY" {
		demandSource = financeOrganicDemandSource
	}
	rules, ruleIDs, err := financeCommissionRulesTx(ctx, tx, facts.CommissionPolicyVersion, facts.Currency)
	if err != nil {
		return outcome, err
	}
	snapshot, err := finance.CalculateCommission(finance.CommissionContext{
		OrderID:      orderID,
		DemandSource: demandSource,
		ProductRef:   financeConsultationProduct,
		Jurisdiction: facts.Jurisdiction,
		AmountMinor:  facts.AmountMinor,
		Currency:     facts.Currency,
	}, rules)
	if errors.Is(err, finance.ErrCommissionPolicyNotConfigured) || (err != nil && demandSource == "") {
		return hold("POLICY_NOT_CONFIGURED")
	}
	if err != nil {
		return hold("POLICY_SNAPSHOT_MISMATCH")
	}
	if snapshot.CommissionMinor != facts.CommissionMinor {
		return hold("POLICY_SNAPSHOT_MISMATCH")
	}

	profile, profileID, err := financeProviderProfileTx(ctx, tx, facts.ProviderConfigID, now)
	if errors.Is(err, ErrFinanceNotFound) {
		return hold("PROVIDER_PROFILE_MISSING")
	}
	if err != nil {
		return outcome, err
	}
	if strings.TrimSpace(facts.CompletionEvidenceRef) == "" {
		return outcome, errors.New("completed consultation lacks provider completion evidence")
	}

	instructionID, err := persistentid.FromRef("finance-settlement", orderID)
	if err != nil {
		return outcome, err
	}
	instruction, err := finance.BuildSettlementInstruction(finance.SettlementContext{
		InstructionID:              instructionID,
		IdempotencyKey:             "settlement:" + orderID,
		OrderID:                    orderID,
		PaymentAttemptID:           facts.AttemptID,
		OriginalProviderID:         facts.ProviderConfigID,
		AmountMinor:                facts.AmountMinor,
		Currency:                   facts.Currency,
		CaptureLedgerEntryRef:      facts.LedgerEntryID,
		CaptureProviderEvidenceRef: facts.CaptureEvidenceRef,
		CompletionEvidenceRef:      facts.CompletionEvidenceRef,
		PayoutBeneficiaryRef:       facts.BeneficiaryRef,
		Commission:                 snapshot,
	}, profile)
	switch {
	case errors.Is(err, finance.ErrProviderCapability):
		return hold("PROVIDER_CAPABILITY_MISSING")
	case errors.Is(err, finance.ErrProviderAffinity):
		return hold("PROVIDER_AFFINITY_VIOLATION")
	case err != nil:
		return outcome, err
	}

	snapshotID, err := persistentid.FromRef("finance-commission", orderID)
	if err != nil {
		return outcome, err
	}
	ruleID := ruleIDs[snapshot.PolicyVersion+"|"+snapshot.DemandSource+"|"+snapshot.ProductRef+"|"+snapshot.Jurisdiction]
	if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_commission_snapshots (id, order_id, rule_id, policy_version, demand_source, product_ref,
  jurisdiction, basis_points, rounding_mode, basis_amount_minor, commission_minor, currency,
  platform_fee_recipient_ref, created_at)
VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		snapshotID, orderID, ruleID, snapshot.PolicyVersion, snapshot.DemandSource, snapshot.ProductRef,
		snapshot.Jurisdiction, snapshot.BasisPoints, snapshot.RoundingMode, snapshot.BasisAmountMinor,
		snapshot.CommissionMinor, snapshot.Currency, snapshot.PlatformFeeRecipientRef, now); err != nil {
		return outcome, fmt.Errorf("persist commission snapshot: %w", err)
	}

	var beneficiaryMinor, feeMinor int64
	for _, allocation := range instruction.Allocations {
		switch allocation.Kind {
		case "SERVICE_BENEFICIARY":
			beneficiaryMinor = allocation.AmountMinor
		case "PLATFORM_FEE":
			feeMinor = allocation.AmountMinor
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_settlement_instructions (id, idempotency_key, order_id, commission_snapshot_id,
  payment_attempt_id, capture_payment_effect_id, capture_ledger_entry_id, provider_config_id,
  provider_profile_id, execution_owner, consultation_session_id, completion_evidence_ref, currency,
  total_amount_minor, beneficiary_ref, beneficiary_amount_minor, platform_fee_recipient_ref,
  platform_fee_minor, created_at)
VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7::uuid,$8::uuid,$9::uuid,$10,$11::uuid,$12,$13,
        $14,$15,$16,$17,$18,$19)`,
		instruction.InstructionID, instruction.IdempotencyKey, orderID, snapshotID, facts.AttemptID,
		facts.EffectID, facts.LedgerEntryID, facts.ProviderConfigID, profileID, instruction.ExecutionOwner,
		facts.SessionID, facts.CompletionEvidenceRef, instruction.Currency, instruction.TotalAmountMinor,
		facts.BeneficiaryRef, beneficiaryMinor, snapshot.PlatformFeeRecipientRef, feeMinor, now); err != nil {
		return outcome, fmt.Errorf("persist settlement instruction: %w", err)
	}

	if err := ensureOutboxEventTx(ctx, tx, financeOutboxEvent(
		"finance-settlement-requested", instruction.InstructionID,
		"finance.settlement_instruction.requested", "settlement/"+instruction.InstructionID,
		facts.CaptureEvidenceRef, now,
		map[string]any{
			"settlement_instruction_id": instruction.InstructionID,
			"idempotency_key":           instruction.IdempotencyKey,
			"order_id":                  orderID,
			"provider_instance_id":      facts.ProviderInstanceID,
			"execution_owner":           instruction.ExecutionOwner,
			"currency":                  instruction.Currency,
			"total_amount_minor":        instruction.TotalAmountMinor,
			"allocations":               instruction.Allocations,
			"policy_version":            instruction.PolicyVersion,
		})); err != nil {
		return outcome, err
	}
	outcome.Status, outcome.SettlementInstructionID = "INSTRUCTED", instruction.InstructionID
	return outcome, nil
}

func financeCommissionRulesTx(ctx context.Context, tx *sql.Tx, policyVersion, currency string) ([]finance.CommissionRule, map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id::text, policy_version, demand_source, product_ref, jurisdiction, basis_points, rounding_mode,
       platform_fee_recipient_ref
  FROM finance_commission_rules
 WHERE policy_version = $1 AND currency = $2`, policyVersion, currency)
	if err != nil {
		return nil, nil, fmt.Errorf("query commission rules: %w", err)
	}
	defer rows.Close()
	var rules []finance.CommissionRule
	ids := map[string]string{}
	for rows.Next() {
		var id string
		var rule finance.CommissionRule
		if err := rows.Scan(&id, &rule.PolicyVersion, &rule.DemandSource, &rule.ProductRef, &rule.Jurisdiction,
			&rule.BasisPoints, &rule.RoundingMode, &rule.PlatformFeeRecipientRef); err != nil {
			return nil, nil, err
		}
		rules = append(rules, rule)
		ids[rule.PolicyVersion+"|"+rule.DemandSource+"|"+rule.ProductRef+"|"+rule.Jurisdiction] = id
	}
	return rules, ids, rows.Err()
}

func financeProviderProfileTx(ctx context.Context, tx *sql.Tx, providerConfigID string, now time.Time) (finance.ProviderExecutionProfile, string, error) {
	var profile finance.ProviderExecutionProfile
	var id, capabilities string
	err := tx.QueryRowContext(ctx, `
SELECT id::text, provider_config_id::text, execution_owner, array_to_string(capabilities, ',')
  FROM finance_provider_execution_profiles
 WHERE provider_config_id = $1::uuid AND effective_from <= $2
 ORDER BY effective_from DESC LIMIT 1`, providerConfigID, now).Scan(&id, &profile.ProviderID, &profile.ExecutionOwner, &capabilities)
	if errors.Is(err, sql.ErrNoRows) {
		return profile, "", ErrFinanceNotFound
	}
	if err != nil {
		return profile, "", fmt.Errorf("read provider execution profile: %w", err)
	}
	profile.Capabilities = strings.Split(capabilities, ",")
	return profile, id, nil
}

func financeOutboxEvent(scope, ref, eventType, aggregateRef, causation string, at time.Time, payload map[string]any) eventspine.EventEnvelope {
	eventID, _ := persistentid.FromRef("outbox-"+scope, ref)
	body, _ := json.Marshal(payload)
	return eventspine.EventEnvelope{
		EventID:          eventID,
		IdempotencyKey:   scope + ":" + ref,
		EventType:        eventType,
		SchemaVersion:    "1",
		AggregateRef:     aggregateRef,
		AggregateVersion: 1,
		OccurredAt:       at,
		ProducedAt:       at,
		Producer:         "finance-r4",
		CorrelationID:    "finance/" + ref,
		CausationID:      causation,
		PayloadJSON:      body,
	}
}

// ------------------------------------------------------- provider evidence

// ApplyFinanceProviderWebhook verifies the provider signature before any
// mutation and then records the evidence exactly once.
func (c *Checker) ApplyFinanceProviderWebhook(ctx context.Context, envelope connector.WebhookEnvelope, keys connector.WebhookPublicKeyResolver) (FinanceEvidenceDecision, error) {
	if c == nil || c.db == nil {
		return FinanceEvidenceDecision{}, errors.New("postgres checker is not initialized")
	}
	if err := connector.VerifyWebhook(envelope, keys); err != nil {
		return FinanceEvidenceDecision{}, err
	}
	var payload FinanceEvidencePayload
	decoder := json.NewDecoder(strings.NewReader(string(envelope.Payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return FinanceEvidenceDecision{}, ErrFinanceEvidenceInvalid
	}
	// Verification covers the entire signed payload, not just the first JSON
	// object. Reject trailing JSON, garbage and unrelated instructions even
	// when the caller bypasses the public HTTP handler and invokes the store.
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return FinanceEvidenceDecision{}, ErrFinanceEvidenceInvalid
	}
	digest := sha256.Sum256(envelope.Payload)
	txCtx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(txCtx, nil)
	if err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("begin finance evidence: %w", err)
	}
	defer tx.Rollback()
	decision, err := recordSequencedFinanceEvidenceTx(txCtx, tx, envelope.ConnectorInstanceID,
		envelope.ExternalEventID, envelope.StreamID, envelope.Sequence,
		"sha256:"+hex.EncodeToString(digest[:]), payload, envelope.OccurredAt.UTC(), time.Now().UTC())
	if err != nil {
		return FinanceEvidenceDecision{}, err
	}
	if err := tx.Commit(); err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("commit finance evidence: %w", err)
	}
	return decision, nil
}

var financeEvidenceKinds = map[string]bool{
	"SETTLEMENT_EXECUTED": true, "PAYOUT_ELIGIBILITY": true, "PAYOUT_EXECUTED": true,
	"PAYOUT_FAILED": true, "STATEMENT_LINE": true,
	"DISPUTE_CHARGEBACK": true, "DISPUTE_REFUND": true, "DISPUTE_REVERSAL": true,
}

func recordFinanceEvidenceTx(ctx context.Context, tx *sql.Tx, providerInstanceID, externalEventID, digest string, payload FinanceEvidencePayload, occurredAt, now time.Time) (FinanceEvidenceDecision, error) {
	// Repository-level tests and internal backfills have no provider stream
	// envelope. Isolate each such event in a deterministic legacy stream; the
	// externally callable webhook path always uses sequenced evidence below.
	return recordFinanceEvidenceInStreamTx(ctx, tx, providerInstanceID, externalEventID,
		"legacy/"+externalEventID, 1, digest, payload, occurredAt, now)
}

func recordSequencedFinanceEvidenceTx(ctx context.Context, tx *sql.Tx, providerInstanceID, externalEventID, streamID string, sequence uint64, digest string, payload FinanceEvidencePayload, occurredAt, now time.Time) (FinanceEvidenceDecision, error) {
	if strings.TrimSpace(streamID) == "" || sequence == 0 || sequence > uint64(1<<63-1) {
		return FinanceEvidenceDecision{}, ErrFinanceEvidenceInvalid
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('finance-stream:' || $1 || ':' || $2))`, providerInstanceID, streamID); err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("lock finance provider stream: %w", err)
	}
	var storedDigest, storedStream string
	var storedSequence int64
	err := tx.QueryRowContext(ctx, `SELECT payload_sha256, COALESCE(stream_id,''), COALESCE(stream_sequence,0)
		FROM finance_provider_evidence WHERE provider_instance_id=$1::uuid AND external_event_id=$2`,
		providerInstanceID, externalEventID).Scan(&storedDigest, &storedStream, &storedSequence)
	if err == nil {
		if storedDigest != digest {
			return FinanceEvidenceDecision{}, ErrFinanceEvidenceConflict
		}
		// Pre-sequencing evidence is immutable legacy history. A byte-identical
		// replay remains idempotent; it cannot be retroactively assigned a stream.
		if storedStream != "" && (storedStream != streamID || storedSequence != int64(sequence)) {
			return FinanceEvidenceDecision{}, ErrFinanceEvidenceConflict
		}
		id, idErr := persistentid.FromRef("finance-evidence", providerInstanceID+"/"+externalEventID)
		if idErr != nil {
			return FinanceEvidenceDecision{}, idErr
		}
		return FinanceEvidenceDecision{EvidenceID: id, Accepted: false, ReasonCode: "FINANCE_EVIDENCE_DUPLICATE"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FinanceEvidenceDecision{}, fmt.Errorf("read finance provider event: %w", err)
	}
	var lastSequence int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence FROM finance_provider_stream_cursors
		WHERE provider_instance_id=$1::uuid AND stream_id=$2 FOR UPDATE`, providerInstanceID, streamID).Scan(&lastSequence)
	if errors.Is(err, sql.ErrNoRows) {
		lastSequence = 0
	} else if err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("read finance provider stream cursor: %w", err)
	}
	switch {
	case int64(sequence) <= lastSequence:
		return FinanceEvidenceDecision{}, finance.ErrProviderEventSequenceStale
	case int64(sequence) != lastSequence+1:
		return FinanceEvidenceDecision{}, finance.ErrProviderEventSequenceGap
	}
	decision, err := recordFinanceEvidenceInStreamTx(ctx, tx, providerInstanceID, externalEventID,
		streamID, sequence, digest, payload, occurredAt, now)
	if err != nil {
		return FinanceEvidenceDecision{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO finance_provider_stream_cursors
		(provider_instance_id, stream_id, last_sequence, last_external_event_id, updated_at)
		VALUES ($1::uuid,$2,$3,$4,$5)
		ON CONFLICT (provider_instance_id, stream_id) DO UPDATE
		SET last_sequence=EXCLUDED.last_sequence,
		    last_external_event_id=EXCLUDED.last_external_event_id,
		    updated_at=EXCLUDED.updated_at`,
		providerInstanceID, streamID, int64(sequence), externalEventID, now)
	if err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("advance finance provider stream cursor: %w", err)
	}
	return decision, nil
}

func recordFinanceEvidenceInStreamTx(ctx context.Context, tx *sql.Tx, providerInstanceID, externalEventID, streamID string, sequence uint64, digest string, payload FinanceEvidencePayload, occurredAt, now time.Time) (FinanceEvidenceDecision, error) {
	payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
	if !financeEvidenceKinds[payload.EvidenceKind] ||
		strings.TrimSpace(providerInstanceID) == "" ||
		strings.TrimSpace(externalEventID) == "" ||
		strings.TrimSpace(streamID) == "" || sequence == 0 || sequence > uint64(1<<63-1) ||
		strings.TrimSpace(payload.SettlementInstructionID) == "" ||
		strings.TrimSpace(payload.ProviderReference) == "" ||
		payload.GrossAmountMinor < 0 || payload.ProviderFeeMinor < 0 ||
		len(payload.Currency) != 3 ||
		(payload.EvidenceKind == "PAYOUT_ELIGIBILITY") != (payload.PayoutEligible != nil) ||
		occurredAt.IsZero() {
		return FinanceEvidenceDecision{}, ErrFinanceEvidenceInvalid
	}
	evidenceID, err := persistentid.FromRef("finance-evidence", providerInstanceID+"/"+externalEventID)
	if err != nil {
		return FinanceEvidenceDecision{}, err
	}
	var compliance any
	if strings.TrimSpace(payload.ComplianceEvidenceRef) != "" {
		compliance = payload.ComplianceEvidenceRef
	}
	var eligible any
	if payload.PayoutEligible != nil {
		eligible = *payload.PayoutEligible
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO finance_provider_evidence (id, provider_instance_id, external_event_id, stream_id, stream_sequence, payload_sha256,
  evidence_kind, settlement_instruction_id, provider_reference, gross_amount_minor, provider_fee_minor,
  currency, payout_eligible, compliance_evidence_ref, occurred_at, received_at)
VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8::uuid,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (provider_instance_id, external_event_id) DO NOTHING`,
		evidenceID, providerInstanceID, externalEventID, streamID, int64(sequence), digest, payload.EvidenceKind,
		payload.SettlementInstructionID, payload.ProviderReference, payload.GrossAmountMinor,
		payload.ProviderFeeMinor, payload.Currency, eligible, compliance, occurredAt, now)
	if err != nil {
		return FinanceEvidenceDecision{}, fmt.Errorf("persist finance evidence: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return FinanceEvidenceDecision{}, err
	}
	if affected == 0 {
		var storedDigest, storedStream string
		var storedSequence int64
		if err := tx.QueryRowContext(ctx, `SELECT payload_sha256, COALESCE(stream_id,''), COALESCE(stream_sequence,0) FROM finance_provider_evidence WHERE id = $1::uuid`, evidenceID).Scan(&storedDigest, &storedStream, &storedSequence); err != nil {
			return FinanceEvidenceDecision{}, fmt.Errorf("read duplicate finance evidence: %w", err)
		}
		if storedDigest != digest {
			return FinanceEvidenceDecision{}, ErrFinanceEvidenceConflict
		}
		if storedStream != "" && (storedStream != streamID || storedSequence != int64(sequence)) {
			return FinanceEvidenceDecision{}, ErrFinanceEvidenceConflict
		}
		return FinanceEvidenceDecision{EvidenceID: evidenceID, Accepted: false, ReasonCode: "FINANCE_EVIDENCE_DUPLICATE"}, nil
	}

	decision := FinanceEvidenceDecision{EvidenceID: evidenceID, Accepted: true, ReasonCode: "FINANCE_EVIDENCE_ACCEPTED"}
	switch payload.EvidenceKind {
	case "PAYOUT_ELIGIBILITY":
		reason, err := decidePayoutTx(ctx, tx, evidenceID, payload, now)
		if err != nil {
			return FinanceEvidenceDecision{}, err
		}
		decision.ReasonCode = reason
	case "DISPUTE_CHARGEBACK", "DISPUTE_REFUND", "DISPUTE_REVERSAL":
		if err := recordDisputeTx(ctx, tx, evidenceID, providerInstanceID, payload, occurredAt); err != nil {
			return FinanceEvidenceDecision{}, err
		}
		decision.ReasonCode = "FINANCE_DISPUTE_RECORDED"
	}
	return decision, nil
}

type financeInstruction struct {
	ID, OrderID, ProviderConfigID, ProviderInstanceID, Currency, BeneficiaryRef, ProfileID, CaptureLedgerEntryID string
	TotalMinor, BeneficiaryMinor                                                                                 int64
}

func loadInstructionTx(ctx context.Context, tx *sql.Tx, id string) (financeInstruction, error) {
	var i financeInstruction
	err := tx.QueryRowContext(ctx, `
SELECT i.id::text, i.order_id::text, i.provider_config_id::text, p.provider_instance_id::text, i.currency,
       i.beneficiary_ref, i.provider_profile_id::text, i.capture_ledger_entry_id::text,
       i.total_amount_minor, i.beneficiary_amount_minor
  FROM finance_settlement_instructions i
  JOIN payment_provider_config_versions p ON p.id = i.provider_config_id
 WHERE i.id = $1::uuid`, id).Scan(&i.ID, &i.OrderID, &i.ProviderConfigID, &i.ProviderInstanceID, &i.Currency,
		&i.BeneficiaryRef, &i.ProfileID, &i.CaptureLedgerEntryID, &i.TotalMinor, &i.BeneficiaryMinor)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrFinanceNotFound
	}
	return i, err
}

func latestEvidenceTx(ctx context.Context, tx *sql.Tx, instructionID, kind string) (id, ref string, gross, fee int64, found bool, err error) {
	err = tx.QueryRowContext(ctx, `
SELECT id::text, provider_reference, gross_amount_minor, provider_fee_minor
  FROM finance_provider_evidence
 WHERE settlement_instruction_id = $1::uuid AND evidence_kind = $2
 ORDER BY occurred_at DESC, received_at DESC, id DESC LIMIT 1`, instructionID, kind).Scan(&id, &ref, &gross, &fee)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, 0, false, nil
	}
	if err != nil {
		return "", "", 0, 0, false, fmt.Errorf("read finance evidence: %w", err)
	}
	return id, ref, gross, fee, true, nil
}

func decidePayoutTx(ctx context.Context, tx *sql.Tx, eligibilityID string, payload FinanceEvidencePayload, now time.Time) (string, error) {
	instruction, err := loadInstructionTx(ctx, tx, payload.SettlementInstructionID)
	if err != nil {
		return "", err
	}
	var alreadyAllowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM finance_payout_decisions WHERE settlement_instruction_id = $1::uuid AND decision = 'ALLOW')`, instruction.ID).Scan(&alreadyAllowed); err != nil {
		return "", err
	}
	if alreadyAllowed {
		return "PAYOUT_ALREADY_INSTRUCTED", nil
	}
	settlementID, settlementRef, _, _, settled, err := latestEvidenceTx(ctx, tx, instruction.ID, "SETTLEMENT_EXECUTED")
	if err != nil {
		return "", err
	}
	var capabilities string
	if err := tx.QueryRowContext(ctx, `SELECT array_to_string(capabilities, ',') FROM finance_provider_execution_profiles WHERE id = $1::uuid`, instruction.ProfileID).Scan(&capabilities); err != nil {
		return "", err
	}
	profile := finance.ProviderExecutionProfile{
		ProviderID: instruction.ProviderConfigID, ExecutionOwner: finance.ExternalExecutionOwner,
		Capabilities: strings.Split(capabilities, ","),
	}
	decisionID, err := persistentid.FromRef("finance-payout-decision", eligibilityID)
	if err != nil {
		return "", err
	}
	reason := "PAYOUT_INSTRUCTION_READY"
	var payout finance.PayoutInstruction
	if !settled {
		reason = "PAYOUT_SETTLEMENT_EVIDENCE_MISSING"
	} else {
		payout, err = finance.BuildPayoutInstruction(finance.PayoutEligibility{
			DecisionID:               decisionID,
			SettlementInstructionID:  instruction.ID,
			OriginalProviderID:       instruction.ProviderConfigID,
			BeneficiaryRef:           instruction.BeneficiaryRef,
			AmountMinor:              instruction.BeneficiaryMinor,
			Currency:                 instruction.Currency,
			ProviderReportedEligible: payload.PayoutEligible != nil && *payload.PayoutEligible,
			ComplianceEvidenceRef:    payload.ComplianceEvidenceRef,
			IdempotencyKey:           "payout:" + instruction.ID,
		}, finance.SettlementEvidence{
			SettlementInstructionID: instruction.ID,
			ProviderID:              instruction.ProviderConfigID,
			ProviderSettlementRef:   settlementRef,
			ProviderEvidenceRef:     "finance-evidence/" + settlementID,
			LedgerEvidenceRef:       instruction.CaptureLedgerEntryID,
		}, profile)
		switch {
		case errors.Is(err, finance.ErrPayoutBlocked):
			reason = "PAYOUT_PROVIDER_ELIGIBILITY_NOT_CONFIRMED"
		case errors.Is(err, finance.ErrProviderCapability):
			reason = "PAYOUT_PROVIDER_CAPABILITY_MISSING"
		case err != nil:
			return "", err
		}
	}
	decision := "BLOCKED"
	var settlementEvidence any
	if settled {
		settlementEvidence = settlementID
	}
	idempotency := "payout-decision:" + eligibilityID
	if reason == "PAYOUT_INSTRUCTION_READY" {
		decision = "ALLOW"
		idempotency = payout.IdempotencyKey
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_payout_decisions (id, idempotency_key, settlement_instruction_id, eligibility_evidence_id,
  settlement_evidence_id, decision, reason_code, execution_owner, beneficiary_ref, amount_minor, currency, decided_at)
VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7,'EXTERNAL_PROVIDER',$8,$9,$10,$11)`,
		decisionID, idempotency, instruction.ID, eligibilityID, settlementEvidence, decision, reason,
		instruction.BeneficiaryRef, instruction.BeneficiaryMinor, instruction.Currency, now); err != nil {
		return "", fmt.Errorf("persist payout decision: %w", err)
	}
	if decision == "ALLOW" {
		if err := ensureOutboxEventTx(ctx, tx, financeOutboxEvent(
			"finance-payout-requested", instruction.ID, "finance.payout_instruction.requested",
			"settlement/"+instruction.ID, "finance-evidence/"+eligibilityID, now,
			map[string]any{
				"payout_decision_id":        decisionID,
				"settlement_instruction_id": instruction.ID,
				"idempotency_key":           payout.IdempotencyKey,
				"provider_instance_id":      instruction.ProviderInstanceID,
				"execution_owner":           payout.ExecutionOwner,
				"beneficiary_ref":           payout.BeneficiaryRef,
				"amount_minor":              payout.AmountMinor,
				"currency":                  payout.Currency,
				"compliance_evidence_ref":   payout.ComplianceEvidenceRef,
			})); err != nil {
			return "", err
		}
	}
	return reason, nil
}

func recordDisputeTx(ctx context.Context, tx *sql.Tx, evidenceID, providerInstanceID string, payload FinanceEvidencePayload, occurredAt time.Time) error {
	instruction, err := loadInstructionTx(ctx, tx, payload.SettlementInstructionID)
	if err != nil {
		return err
	}
	kind := strings.TrimPrefix(payload.EvidenceKind, "DISPUTE_")
	ledgerID, err := persistentid.FromRef("finance-dispute-ledger", evidenceID)
	if err != nil {
		return err
	}
	effectID, err := persistentid.FromRef("finance-dispute", evidenceID)
	if err != nil {
		return err
	}
	effect, err := finance.NewDisputeEffect(finance.DisputeEffect{
		EffectID:            effectID,
		EconomicRef:         "settlement/" + instruction.ID,
		OriginalProviderID:  instruction.ProviderInstanceID,
		ProviderID:          providerInstanceID,
		Kind:                finance.DisputeKind(kind),
		AmountMinor:         payload.GrossAmountMinor,
		Currency:            payload.Currency,
		ProviderEvidenceRef: "finance-evidence/" + evidenceID,
		LedgerEvidenceRef:   ledgerID,
		AuditEvidenceRef:    "provider/" + providerInstanceID + "/" + payload.ProviderReference,
		ExecutionOwner:      finance.ExternalExecutionOwner,
	})
	if err != nil {
		return err
	}
	// The provider executes the monetary reversal; APGIC records the
	// mirrored non-custodial economic fact with a new, linked ledger entry.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO ledger_entries (id, debit_account_ref, credit_account_ref, amount_minor, currency,
  provider_evidence_ref, economic_event_ref, correlation_id, occurred_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ledgerID, instruction.BeneficiaryRef, "external-provider/"+providerInstanceID+"/payer",
		effect.AmountMinor, effect.Currency, effect.ProviderEvidenceRef,
		"finance-dispute/"+evidenceID, "finance/"+instruction.ID, occurredAt); err != nil {
		return fmt.Errorf("persist dispute ledger fact: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_dispute_effects (id, idempotency_key, settlement_instruction_id, provider_evidence_id, kind,
  amount_minor, currency, ledger_entry_id, audit_ref, execution_owner, occurred_at)
VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7,$8::uuid,$9,$10,$11)`,
		effect.EffectID, "dispute:"+evidenceID, instruction.ID, evidenceID, string(effect.Kind),
		effect.AmountMinor, effect.Currency, ledgerID, effect.AuditEvidenceRef, effect.ExecutionOwner, occurredAt); err != nil {
		return fmt.Errorf("persist dispute effect: %w", err)
	}
	return ensureOutboxEventTx(ctx, tx, financeOutboxEvent(
		"finance-dispute-recorded", evidenceID, "finance.dispute_effect.recorded",
		"settlement/"+instruction.ID, effect.ProviderEvidenceRef, occurredAt,
		map[string]any{
			"dispute_effect_id":         effect.EffectID,
			"settlement_instruction_id": instruction.ID,
			"kind":                      string(effect.Kind),
			"amount_minor":              effect.AmountMinor,
			"currency":                  effect.Currency,
			"execution_owner":           effect.ExecutionOwner,
		}))
}

// ----------------------------------------------------------- reconciliation

func (c *Checker) ReconcileFinanceProvider(ctx context.Context, providerInstanceID string, periodStart, periodEnd time.Time) (FinanceReconciliationReport, error) {
	if c == nil || c.db == nil {
		return FinanceReconciliationReport{}, errors.New("postgres checker is not initialized")
	}
	txCtx, cancel := context.WithTimeout(ctx, 4*financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(txCtx, nil)
	if err != nil {
		return FinanceReconciliationReport{}, fmt.Errorf("begin reconciliation: %w", err)
	}
	defer tx.Rollback()
	report, err := reconcileProviderTx(txCtx, tx, providerInstanceID, periodStart.UTC(), periodEnd.UTC(), time.Now().UTC())
	if err != nil {
		return FinanceReconciliationReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return FinanceReconciliationReport{}, fmt.Errorf("commit reconciliation: %w", err)
	}
	return report, nil
}

func reconcileProviderTx(ctx context.Context, tx *sql.Tx, providerInstanceID string, periodStart, periodEnd, now time.Time) (FinanceReconciliationReport, error) {
	if !periodEnd.After(periodStart) {
		return FinanceReconciliationReport{}, finance.ErrInvalidReconciliation
	}
	runID, err := persistentid.FromRef("finance-reconciliation-run",
		providerInstanceID+"/"+strconv.FormatInt(periodStart.UnixNano(), 10)+"/"+strconv.FormatInt(periodEnd.UnixNano(), 10))
	if err != nil {
		return FinanceReconciliationReport{}, err
	}
	report := FinanceReconciliationReport{RunID: runID}
	result, err := tx.ExecContext(ctx, `
INSERT INTO finance_reconciliation_runs (id, provider_instance_id, period_start, period_end, started_at)
VALUES ($1::uuid,$2::uuid,$3,$4,$5) ON CONFLICT (id) DO NOTHING`, runID, providerInstanceID, periodStart, periodEnd, now)
	if err != nil {
		return report, fmt.Errorf("persist reconciliation run: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// Idempotent replay: report the recorded outcome, never rewrite it.
		err := tx.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE outcome = 'MATCHED'), count(*) FILTER (WHERE outcome = 'MISMATCH')
  FROM finance_reconciliation_results WHERE run_id = $1::uuid`, runID).Scan(&report.Matched, &report.Mismatched)
		return report, err
	}

	rows, err := tx.QueryContext(ctx, `
SELECT i.id::text
  FROM finance_settlement_instructions i
  JOIN payment_provider_config_versions p ON p.id = i.provider_config_id
 WHERE p.provider_instance_id = $1::uuid AND i.created_at >= $2 AND i.created_at < $3
 ORDER BY i.created_at, i.id`, providerInstanceID, periodStart, periodEnd)
	if err != nil {
		return report, fmt.Errorf("query reconciliation scope: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return report, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return report, err
	}

	for _, instructionID := range ids {
		instruction, err := loadInstructionTx(ctx, tx, instructionID)
		if err != nil {
			return report, err
		}
		statementID, _, statementGross, statementFee, hasStatement, err := latestEvidenceTx(ctx, tx, instructionID, "STATEMENT_LINE")
		if err != nil {
			return report, err
		}
		settlementID, _, _, settlementFee, hasSettlement, err := latestEvidenceTx(ctx, tx, instructionID, "SETTLEMENT_EXECUTED")
		if err != nil {
			return report, err
		}
		payoutID, _, _, _, hasPayout, err := latestEvidenceTx(ctx, tx, instructionID, "PAYOUT_EXECUTED")
		if err != nil {
			return report, err
		}
		if !hasStatement || !hasSettlement || !hasPayout {
			report.Pending++
			continue
		}
		var statementCurrency string
		if err := tx.QueryRowContext(ctx, `SELECT currency FROM finance_provider_evidence WHERE id = $1::uuid`, statementID).Scan(&statementCurrency); err != nil {
			return report, err
		}
		outcome, err := finance.Reconcile(finance.ExpectedFinancialState{
			EconomicRef:           "settlement/" + instructionID,
			OriginalProviderID:    instruction.ProviderInstanceID,
			GrossMinor:            instruction.TotalMinor,
			ProviderFeeMinor:      settlementFee,
			Currency:              instruction.Currency,
			LedgerEvidenceRef:     "ledger/" + instruction.CaptureLedgerEntryID,
			SettlementEvidenceRef: "finance-evidence/" + settlementID,
			PayoutEvidenceRef:     "finance-evidence/" + payoutID,
		}, finance.ProviderStatementFact{
			ProviderID:           providerInstanceID,
			EconomicRef:          "settlement/" + instructionID,
			GrossMinor:           statementGross,
			ProviderFeeMinor:     statementFee,
			Currency:             statementCurrency,
			StatementEvidenceRef: "finance-evidence/" + statementID,
		})
		if err != nil {
			return report, err
		}
		resultID, err := persistentid.FromRef("finance-reconciliation-result", runID+"/"+instructionID)
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_reconciliation_results (id, run_id, settlement_instruction_id, statement_evidence_id,
  settlement_evidence_id, payout_evidence_id, outcome, expected_gross_minor, provider_gross_minor,
  expected_fee_minor, provider_fee_minor, currency, decided_at)
VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7,$8,$9,$10,$11,$12,$13)`,
			resultID, runID, instructionID, statementID, settlementID, payoutID, string(outcome.Outcome),
			instruction.TotalMinor, statementGross, settlementFee, statementFee, instruction.Currency, now); err != nil {
			return report, fmt.Errorf("persist reconciliation result: %w", err)
		}
		if outcome.Discrepancy == nil {
			report.Matched++
			continue
		}
		report.Mismatched++
		discrepancyID, err := persistentid.FromRef("finance-discrepancy", resultID)
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_reconciliation_discrepancies (id, result_id, kind, owner, status, evidence_refs, created_at)
VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7)`,
			discrepancyID, resultID, outcome.Discrepancy.Kind, outcome.Discrepancy.Owner,
			string(outcome.Discrepancy.Status), "{"+strings.Join(quoteArray(outcome.Discrepancy.EvidenceRefs), ",")+"}", now); err != nil {
			return report, fmt.Errorf("persist reconciliation discrepancy: %w", err)
		}
		if err := ensureOutboxEventTx(ctx, tx, financeOutboxEvent(
			"finance-discrepancy-opened", discrepancyID, "finance.reconciliation_discrepancy.opened",
			"settlement/"+instructionID, "finance-evidence/"+statementID, now,
			map[string]any{
				"discrepancy_id":            discrepancyID,
				"settlement_instruction_id": instructionID,
				"kind":                      outcome.Discrepancy.Kind,
				"owner":                     outcome.Discrepancy.Owner,
			})); err != nil {
			return report, err
		}
	}
	return report, nil
}

func quoteArray(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
	}
	return quoted
}

// FinanceProviderInstances lists provider instances with settlement history,
// i.e. the scope the reconciliation worker must cover.
func (c *Checker) FinanceProviderInstances(ctx context.Context) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(queryCtx, `
SELECT DISTINCT p.provider_instance_id::text
  FROM finance_settlement_instructions i
  JOIN payment_provider_config_versions p ON p.id = i.provider_config_id
 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ----------------------------------------------------------------- read API

// ReadFinanceOrderStatus returns the owner-scoped financial status. Only the
// booking client or the booked specialist may read it; others get not-found.
func (c *Checker) ReadFinanceOrderStatus(ctx context.Context, orderID, identityID string) (*FinanceOrderStatus, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	txCtx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(txCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return readFinanceOrderStatusTx(txCtx, tx, orderID, identityID)
}

func readFinanceOrderStatusTx(ctx context.Context, tx *sql.Tx, orderID, identityID string) (*FinanceOrderStatus, error) {
	if !financeUUIDPattern.MatchString(orderID) || !financeUUIDPattern.MatchString(identityID) {
		return nil, nil
	}
	status := &FinanceOrderStatus{OrderID: orderID, ExecutionOwner: finance.ExternalExecutionOwner, DiscrepancyStatus: "NONE"}
	err := tx.QueryRowContext(ctx, `
SELECT o.currency, o.amount_minor
  FROM orders o
  JOIN bookings b ON b.id = o.booking_id
  JOIN booking_slots s ON s.id = b.slot_id
 WHERE o.id = $1::uuid AND (b.client_identity_id = $2::uuid OR s.specialist_identity_id = $2::uuid)`,
		orderID, identityID).Scan(&status.Currency, &status.TotalAmountMinor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read finance order: %w", err)
	}

	revenue := finance.RevenueEvidence{}
	var paymentRef, ledgerRef sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT e.provider_evidence_ref, e.ledger_entry_id::text
  FROM payment_attempts a JOIN payment_effects e ON e.attempt_id = a.id AND e.effect_kind = 'CAPTURED'
 WHERE a.order_id = $1::uuid LIMIT 1`, orderID).Scan(&paymentRef, &ledgerRef); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	revenue.PaymentEvidenceRef, revenue.LedgerEvidenceRef = paymentRef.String, ledgerRef.String

	var instructionID, completionRef, policyVersion, snapshotID string
	var fee, beneficiary int64
	err = tx.QueryRowContext(ctx, `
SELECT i.id::text, i.completion_evidence_ref, c.policy_version, c.id::text, i.platform_fee_minor, i.beneficiary_amount_minor
  FROM finance_settlement_instructions i JOIN finance_commission_snapshots c ON c.id = i.commission_snapshot_id
 WHERE i.order_id = $1::uuid`, orderID).Scan(&instructionID, &completionRef, &policyVersion, &snapshotID, &fee, &beneficiary)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		status.SettlementStatus, status.PayoutStatus, status.ReconciliationOutcome = "NOT_INSTRUCTED", "NOT_ELIGIBLE", "NOT_RECONCILED"
		rows, err := tx.QueryContext(ctx, `SELECT reason_code FROM finance_settlement_holds WHERE order_id = $1::uuid ORDER BY reason_code`, orderID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var reason string
			if err := rows.Scan(&reason); err != nil {
				rows.Close()
				return nil, err
			}
			status.SettlementHoldReasons = append(status.SettlementHoldReasons, reason)
		}
		rows.Close()
		if len(status.SettlementHoldReasons) > 0 {
			status.SettlementStatus = "HELD"
		}
		result, _ := finance.EvaluateRevenueAssurance(revenue)
		status.RevenueAssurance, status.RevenueReasonCode = result.Outcome, result.ReasonCode
		return status, nil
	case err != nil:
		return nil, err
	}
	status.CommissionPolicyVersion = policyVersion
	status.PlatformFeeMinor, status.BeneficiaryAmountMinor = &fee, &beneficiary
	revenue.FulfillmentEvidenceRef = completionRef
	revenue.CommissionEvidenceRef = "commission/" + snapshotID

	status.SettlementStatus = "INSTRUCTED"
	if id, _, _, _, ok, err := latestEvidenceTx(ctx, tx, instructionID, "SETTLEMENT_EXECUTED"); err != nil {
		return nil, err
	} else if ok {
		status.SettlementStatus = "PROVIDER_SETTLED"
		revenue.SettlementEvidenceRef = "finance-evidence/" + id
	}

	status.PayoutStatus = "AWAITING_PROVIDER_ELIGIBILITY"
	var decision, reason string
	err = tx.QueryRowContext(ctx, `
SELECT decision, reason_code FROM finance_payout_decisions
 WHERE settlement_instruction_id = $1::uuid ORDER BY (decision = 'ALLOW') DESC, decided_at DESC LIMIT 1`, instructionID).Scan(&decision, &reason)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		status.PayoutReasonCode = reason
		if decision == "ALLOW" {
			status.PayoutStatus = "INSTRUCTED"
		} else {
			status.PayoutStatus = "BLOCKED"
		}
	}
	if id, _, _, _, ok, err := latestEvidenceTx(ctx, tx, instructionID, "PAYOUT_EXECUTED"); err != nil {
		return nil, err
	} else if ok {
		status.PayoutStatus = "PROVIDER_PAID_OUT"
		revenue.PayoutEvidenceRef = "finance-evidence/" + id
	}

	status.ReconciliationOutcome = "NOT_RECONCILED"
	var outcome string
	var discrepancyID sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT r.outcome, d.id::text
  FROM finance_reconciliation_results r
  LEFT JOIN finance_reconciliation_discrepancies d ON d.result_id = r.id
 WHERE r.settlement_instruction_id = $1::uuid ORDER BY r.decided_at DESC, r.id DESC LIMIT 1`, instructionID).Scan(&outcome, &discrepancyID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		status.ReconciliationOutcome = outcome
		revenue.ReconciliationOutcome = finance.ReconciliationOutcome(outcome)
		if discrepancyID.Valid {
			revenue.DiscrepancyEvidenceRef = "discrepancy/" + discrepancyID.String
		}
	}
	if status.DiscrepancyStatus, status.DiscrepancyResolution, err = financeDiscrepancyStatusTx(ctx, tx, instructionID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(amount_minor), 0) FROM finance_dispute_effects WHERE settlement_instruction_id = $1::uuid`, instructionID).Scan(&status.DisputedAmountMinor); err != nil {
		return nil, err
	}
	result, _ := finance.EvaluateRevenueAssurance(revenue)
	status.RevenueAssurance, status.RevenueReasonCode = result.Outcome, result.ReasonCode
	return status, nil
}
