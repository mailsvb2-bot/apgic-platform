package runtimepostgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyBankCaptureResult is the result of one committed canonical Commerce
// capture. Applied=false means a verified retry found the existing purchase.
type AcademyBankCaptureResult struct {
	OrderID          string
	PaymentAttemptID string
	EntitlementID    string
	Applied          bool
	Active           bool
}

type academyBankCaptureBasis struct {
	AttemptID   string
	Reference   string
	BuyerID     string
	ProductID   string
	ProviderID  string
	Beneficiary string
	AmountMinor int64
	State       string
}

// AcademyCommitBankCapture requires an authentic signed bank CONFIRMED webhook
// AND an independently fetched CONFIRMED GetState for exactly the durable
// OrderId/PaymentId/terminal/amount. Neither one alone mints paid access.
// It atomically commits the canonical webhook receipt, attempt transition,
// ledger, CAPTURED effect, verified transaction, entitlement and audit record.
// The bank request is outside the SQL transaction; the attempt is rechecked
// under FOR UPDATE before any economic write.
func (c *Checker) AcademyCommitBankCapture(ctx context.Context, orderID string,
	signedNotification []byte, bank *payments.TBankAcquiring) (AcademyBankCaptureResult, error) {
	if ctx == nil || !academyMediaUUID(orderID) || len(signedNotification) == 0 ||
		len(signedNotification) > 16384 || bank == nil {
		return AcademyBankCaptureResult{}, academy.ErrInvalid
	}
	if c == nil || c.db == nil {
		return AcademyBankCaptureResult{}, academy.ErrUnavailable
	}
	basis, err := c.academyBankCaptureBasis(ctx, orderID, false)
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	terminal, paymentID, ok := academyParseTBankReference(basis.Reference)
	if !ok {
		return AcademyBankCaptureResult{}, academy.ErrDenied
	}
	expected := payments.TBankCaptureExpectation{
		TerminalKey: terminal, OrderID: orderID, PaymentID: paymentID, AmountMinor: basis.AmountMinor,
	}
	proofCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := bank.VerifyCapturedNotification(signedNotification, expected); err != nil {
		return AcademyBankCaptureResult{}, academy.ErrDenied
	}
	confirmed, err := bank.GetState(proofCtx, expected)
	if err != nil {
		// Valid signed payment notifications must be retried when the bank
		// cannot prove its current state. Never commit a payment on outage.
		return AcademyBankCaptureResult{}, academy.ErrUnavailable
	}
	if confirmed.OrderID != orderID || confirmed.PaymentID != paymentID ||
		confirmed.AmountMinor != basis.AmountMinor || confirmed.ProviderStatus != "CONFIRMED" {
		return AcademyBankCaptureResult{}, academy.ErrDenied
	}
	dbctx, dbCancel, tx, err := academyBegin(c)
	defer dbCancel()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	defer tx.Rollback()
	// Lock after the remote call. A concurrently revoked or reassigned
	// transaction can never be admitted using a stale pre-network read.
	current, err := academyBankCaptureBasisTx(dbctx, tx, orderID, true)
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	if current.AttemptID != basis.AttemptID || current.Reference != basis.Reference ||
		current.BuyerID != basis.BuyerID || current.ProductID != basis.ProductID ||
		current.ProviderID != basis.ProviderID || current.AmountMinor != basis.AmountMinor {
		return AcademyBankCaptureResult{}, academy.ErrConflict
	}
	outcome := AcademyBankCaptureResult{OrderID: orderID, PaymentAttemptID: current.AttemptID}
	if current.State == "SUCCEEDED" {
		var state string
		err = tx.QueryRowContext(dbctx, `SELECT t.id::text,t.state
 FROM store_entitlements t JOIN store_transaction_verifications v ON v.id=t.verification_id
 WHERE v.payment_attempt_id=$1::uuid AND v.order_id=$2::uuid
 AND v.identity_id=$3::uuid AND v.product_ref=$4`,
			current.AttemptID, orderID, current.BuyerID, current.ProductID).
			Scan(&outcome.EntitlementID, &state)
		if err != nil {
			return AcademyBankCaptureResult{}, err
		}
		outcome.Active = state == "ACTIVE"
		if err = tx.Commit(); err != nil {
			return AcademyBankCaptureResult{}, err
		}
		return outcome, nil
	}
	if current.State != "SENT" && current.State != "PENDING" && current.State != "AMBIGUOUS" {
		return AcademyBankCaptureResult{}, academy.ErrState
	}
	receiptID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	ledgerID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	effectID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	verificationID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	entitlementID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	auditID, err := academyID()
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	digest := sha256.Sum256(signedNotification)
	eventID := "tbank/captured/" + terminal + "/" + paymentID
	evidenceRef := "tbank/confirmed/" + terminal + "/" + paymentID
	var accepted bool
	var reason string
	err = tx.QueryRowContext(dbctx, `SELECT accepted,reason_code FROM apgic_record_payment_webhook(
 $1::uuid,$2::uuid,$3,$4,now())`,
		receiptID, current.ProviderID, eventID, "sha256:"+hex.EncodeToString(digest[:])).Scan(&accepted, &reason)
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	if !accepted || reason != "PAY_WEBHOOK_ACCEPTED" {
		return AcademyBankCaptureResult{}, academy.ErrConflict
	}
	result, err := tx.ExecContext(dbctx, `UPDATE payment_attempts
 SET state='SUCCEEDED',provider_evidence_ref=$1,updated_at=now()
 WHERE id=$2::uuid AND order_id=$3::uuid
 AND provider_reference=$4 AND state IN ('SENT','PENDING','AMBIGUOUS')`,
		evidenceRef, current.AttemptID, orderID, current.Reference)
	if err != nil {
		return AcademyBankCaptureResult{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return AcademyBankCaptureResult{}, academy.ErrConflict
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO ledger_entries(
 id,debit_account_ref,credit_account_ref,amount_minor,currency,provider_evidence_ref,
 economic_event_ref,correlation_id,occurred_at)
 VALUES($1::uuid,$2,$3,$4,'RUB',$5,$6,$7,now())`,
		ledgerID, "external-psp/tbank/"+terminal, current.Beneficiary, current.AmountMinor,
		evidenceRef, "payment/"+current.AttemptID, "order/"+orderID)
	if err != nil {
		return AcademyBankCaptureResult{}, fmt.Errorf("canonical bank ledger: %w", err)
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO payment_effects(
 id,receipt_id,attempt_id,effect_kind,provider_evidence_ref,ledger_entry_id,occurred_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,'CAPTURED',$4,$5::uuid,now())`,
		effectID, receiptID, current.AttemptID, evidenceRef, ledgerID)
	if err != nil {
		return AcademyBankCaptureResult{}, fmt.Errorf("canonical bank effect: %w", err)
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO store_transaction_verifications(
 id,identity_id,order_id,payment_attempt_id,payment_effect_id,ledger_entry_id,
 provider_instance_id,external_transaction_id,product_ref,entitlement_kind,
 policy_snapshot_id,provider_evidence_ref,verified_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7::uuid,
 $8,$9,'ONE_TIME',NULL,$10,now())`,
		verificationID, current.BuyerID, orderID, current.AttemptID, effectID,
		ledgerID, current.ProviderID, current.Reference, current.ProductID, evidenceRef)
	if err != nil {
		return AcademyBankCaptureResult{}, fmt.Errorf("canonical bank verified purchase: %w", err)
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO store_entitlements(
 id,identity_id,product_ref,kind,state,verification_id,activated_at,updated_at)
 VALUES($1::uuid,$2::uuid,$3,'ONE_TIME','ACTIVE',$4::uuid,now(),now())`,
		entitlementID, current.BuyerID, current.ProductID, verificationID)
	if err != nil {
		return AcademyBankCaptureResult{}, fmt.Errorf("canonical bank entitlement: %w", err)
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO audit_records(
 id,actor_id,action,scope,resource_ref,new_state,reason,policy_version,correlation_id)
 VALUES($1::uuid,'provider/tbank','payment.capture_committed',$2,$3,
 jsonb_build_object('order_id',$4,'attempt_id',$5,'entitlement_id',$6),
 'signed_webhook_and_bank_getstate_confirmed','academy-tbank-capture-v1',$7)`,
		auditID, "order/"+orderID, "payment/"+current.AttemptID, orderID,
		current.AttemptID, entitlementID, "order/"+orderID)
	if err != nil {
		return AcademyBankCaptureResult{}, fmt.Errorf("canonical bank capture audit: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return AcademyBankCaptureResult{}, err
	}
	outcome.EntitlementID = entitlementID
	outcome.Applied = true
	outcome.Active = true
	return outcome, nil
}

func academyParseTBankReference(ref string) (terminal, paymentID string, ok bool) {
	parts := strings.Split(ref, "/")
	if len(parts) != 3 || parts[0] != "tbank" || parts[1] == "" || parts[2] == "" ||
		len(parts[1]) > 128 || len(parts[2]) > 32 {
		return "", "", false
	}
	for _, c := range parts[2] {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}
	return parts[1], parts[2], true
}

func (c *Checker) academyBankCaptureBasis(ctx context.Context, orderID string, lock bool) (academyBankCaptureBasis, error) {
	if lock {
		return academyBankCaptureBasis{}, academy.ErrInvalid
	}
	var b academyBankCaptureBasis
	err := c.db.QueryRowContext(ctx, academyBankCaptureSQL(false), orderID).Scan(
		&b.AttemptID, &b.Reference, &b.BuyerID, &b.ProductID, &b.ProviderID,
		&b.Beneficiary, &b.AmountMinor, &b.State)
	if errors.Is(err, sql.ErrNoRows) {
		return academyBankCaptureBasis{}, academy.ErrDenied
	}
	return b, err
}

func academyBankCaptureBasisTx(ctx context.Context, tx *sql.Tx, orderID string, lock bool) (academyBankCaptureBasis, error) {
	var b academyBankCaptureBasis
	err := tx.QueryRowContext(ctx, academyBankCaptureSQL(lock), orderID).Scan(
		&b.AttemptID, &b.Reference, &b.BuyerID, &b.ProductID, &b.ProviderID,
		&b.Beneficiary, &b.AmountMinor, &b.State)
	if errors.Is(err, sql.ErrNoRows) {
		return academyBankCaptureBasis{}, academy.ErrDenied
	}
	return b, err
}

func academyBankCaptureSQL(lock bool) string {
	sqlText := `SELECT pa.id::text,pa.provider_reference,
 o.direct_buyer_identity_id::text,o.product_id::text,
 cfg.provider_instance_id::text,o.payout_beneficiary_ref,
 pa.amount_minor,pa.state
 FROM orders o JOIN payment_attempts pa ON pa.order_id=o.id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 WHERE o.id=$1::uuid AND o.booking_id IS NULL
 AND o.direct_buyer_identity_id IS NOT NULL
 AND o.product_id IS NOT NULL AND ci.provider_kind='TBANK'
 AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER'
 AND route.selected_method_code='BANK_CARD'
 AND pa.provider_reference LIKE 'tbank/%'
 ORDER BY pa.created_at DESC LIMIT 1`
	if lock {
		sqlText += ` FOR UPDATE OF pa`
	}
	return sqlText
}
