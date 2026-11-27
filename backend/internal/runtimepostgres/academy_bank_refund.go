package runtimepostgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

type academyBankRefundBasis struct {
	ID, Attempt, Reference, Provider, Debit, Credit, State string
	Amount                                                 int64
}

const academyBankRefundQuery = `SELECT r.id::text,pa.id::text,pa.provider_reference,
 cfg.provider_instance_id::text,l.credit_account_ref,l.debit_account_ref,r.state,r.amount_minor
 FROM refund_requests r
 JOIN orders o ON o.id=r.order_id
 JOIN payment_attempts pa ON pa.id=r.original_payment_attempt_id AND pa.order_id=o.id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id AND cfg.id=r.original_provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 JOIN payment_effects e ON e.attempt_id=pa.id AND e.effect_kind='CAPTURED'
 JOIN ledger_entries l ON l.id=e.ledger_entry_id
 WHERE o.id=$1::uuid AND o.booking_id IS NULL AND r.booking_id IS NULL
 AND o.direct_buyer_identity_id IS NOT NULL AND pa.state='SUCCEEDED'
 AND ci.provider_kind='TBANK' AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER'
 AND route.selected_method_code='BANK_CARD' AND r.amount_minor=pa.amount_minor
 AND r.currency='RUB' AND pa.currency='RUB'
 AND r.state IN ('SENT','PENDING','AMBIGUOUS','SUCCEEDED')
 AND r.provider_reference=pa.provider_reference`

func scanAcademyBankRefund(row *sql.Row) (academyBankRefundBasis, error) {
	var b academyBankRefundBasis
	err := row.Scan(&b.ID, &b.Attempt, &b.Reference, &b.Provider, &b.Debit, &b.Credit, &b.State, &b.Amount)
	if errors.Is(err, sql.ErrNoRows) {
		return b, academy.ErrDenied
	}
	return b, err
}

// AcademyCommitBankRefund records a full, approved refund only after two
// independent bank signals. It reuses the R2 reversal owner and never issues
// another Cancel or grants access.
func (c *Checker) AcademyCommitBankRefund(ctx context.Context, orderID string,
	raw []byte, bank *payments.TBankAcquiring) error {
	if ctx == nil || !academyMediaUUID(orderID) || len(raw) == 0 || len(raw) > 16384 || bank == nil {
		return academy.ErrInvalid
	}
	if c == nil || c.db == nil {
		return academy.ErrUnavailable
	}
	b, err := scanAcademyBankRefund(c.db.QueryRowContext(ctx, academyBankRefundQuery, orderID))
	if err != nil {
		return err
	}
	terminal, paymentID, ok := academyParseTBankReference(b.Reference)
	if !ok {
		return academy.ErrDenied
	}
	expected := payments.TBankCaptureExpectation{TerminalKey: terminal, OrderID: orderID, PaymentID: paymentID, AmountMinor: b.Amount}
	proofCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err = bank.ConfirmRefundNotificationWithGetState(proofCtx, raw, expected); err != nil {
		return academy.ErrUnavailable
	}
	dbctx, dbCancel, tx, err := academyBegin(c)
	defer dbCancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := scanAcademyBankRefund(tx.QueryRowContext(dbctx, academyBankRefundQuery+" FOR UPDATE OF r", orderID))
	if err != nil {
		return err
	}
	b.State = current.State
	if b != current {
		return academy.ErrConflict
	}
	if current.State == "SUCCEEDED" {
		var exists bool
		if err = tx.QueryRowContext(dbctx, "SELECT EXISTS(SELECT 1 FROM refund_effects WHERE refund_request_id=$1::uuid)", b.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return academy.ErrConflict
		}
		return tx.Commit()
	}
	receipt, err := academyID()
	if err != nil {
		return err
	}
	ledger, err := academyID()
	if err != nil {
		return err
	}
	effect, err := academyID()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	evidence := "tbank/refunded/" + terminal + "/" + paymentID
	var accepted bool
	var reason string
	err = tx.QueryRowContext(dbctx, "SELECT accepted,reason_code FROM apgic_record_payment_webhook($1::uuid,$2::uuid,$3,$4,now())",
		receipt, b.Provider, evidence, "sha256:"+hex.EncodeToString(digest[:])).Scan(&accepted, &reason)
	if err != nil {
		return err
	}
	if !accepted || reason != "PAY_WEBHOOK_ACCEPTED" {
		return academy.ErrConflict
	}
	_, err = tx.ExecContext(dbctx, `INSERT INTO ledger_entries
 (id,debit_account_ref,credit_account_ref,amount_minor,currency,provider_evidence_ref,economic_event_ref,correlation_id,occurred_at)
 VALUES($1::uuid,$2,$3,$4,'RUB',$5,$6,$7,now())`,
		ledger, b.Debit, b.Credit, b.Amount, evidence, "refund/"+b.ID, "order/"+orderID)
	if err != nil {
		return err
	}
	var applied bool
	err = tx.QueryRowContext(dbctx, "SELECT applied,reason_code FROM apgic_apply_refund_success($1::uuid,$2::uuid,$3,$4::uuid,$5::uuid,now())",
		b.ID, receipt, evidence, ledger, effect).Scan(&applied, &reason)
	if err != nil {
		return err
	}
	if !applied || reason != "REFUND_APPLIED" {
		return academy.ErrConflict
	}
	return tx.Commit()
}
