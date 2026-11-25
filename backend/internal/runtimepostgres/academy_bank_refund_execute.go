package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyExecuteApprovedBankRefund is a server-side execution seam, not a
// buyer authorization endpoint. Only an existing policy-approved Commerce
// refund request can be executed. The durable claim precedes the PSP call;
// uncertain outcomes are never retried through Cancel.
func (c *Checker) AcademyExecuteApprovedBankRefund(ctx context.Context, refundID string) error {
	if ctx == nil || !academyMediaUUID(refundID) {
		return academy.ErrInvalid
	}
	if c == nil || c.db == nil || os.Getenv("APGIC_ACADEMY_TBANK_REFUND_ENABLED") != "true" {
		return academy.ErrUnavailable
	}
	const basis = `SELECT pa.provider_reference,r.amount_minor,seller.terminal_key
 FROM refund_requests r
 JOIN orders o ON o.id=r.order_id
 JOIN payment_attempts pa ON pa.id=r.original_payment_attempt_id AND pa.order_id=o.id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id AND cfg.id=r.original_provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 JOIN academy_bank_seller_bindings seller ON seller.provider_config_id=cfg.id
 AND seller.product_owner_ref=o.product_owner_ref AND seller.credential_version_ref=cfg.credential_version_ref
 WHERE r.id=$1::uuid AND r.state='REQUESTED' AND r.provider_reference IS NULL
 AND r.policy_decision='ALLOW' AND r.execution_owner='EXTERNAL_PROVIDER'
 AND r.booking_id IS NULL AND o.booking_id IS NULL AND o.direct_buyer_identity_id IS NOT NULL
 AND pa.state='SUCCEEDED' AND r.amount_minor=pa.amount_minor AND r.currency='RUB' AND pa.currency='RUB'
 AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER' AND route.selected_method_code='BANK_CARD'
 AND ci.provider_kind='TBANK' AND ci.status='ACTIVE' AND cfg.status='ACTIVE'
 AND seller.revoked_at IS NULL`
	var reference, terminal string
	var amount int64
	err := c.db.QueryRowContext(ctx, basis, refundID).Scan(&reference, &amount, &terminal)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrConflict
	}
	if err != nil {
		return err
	}
	refTerminal, paymentID, ok := academyParseTBankReference(reference)
	if !ok || terminal != refTerminal || len(paymentID) > 20 || amount <= 0 || amount > 10000000000 {
		return academy.ErrDenied
	}
	bank, err := c.AcademyBankForTerminal(ctx, terminal)
	if err != nil {
		return err
	}
	dbctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRef, currentTerminal string
	var currentAmount int64
	err = tx.QueryRowContext(dbctx, basis+" FOR UPDATE OF r", refundID).Scan(&currentRef, &currentAmount, &currentTerminal)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrConflict
	}
	if err != nil {
		return err
	}
	if currentRef != reference || currentAmount != amount || currentTerminal != terminal {
		return academy.ErrConflict
	}
	result, err := tx.ExecContext(dbctx, `UPDATE refund_requests SET state='SENT',
 provider_reference=$2,updated_at=now() WHERE id=$1::uuid AND state='REQUESTED'`, refundID, reference)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return academy.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	proofCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	_, err = bank.Cancel(proofCtx, payments.TBankCancelRequest{PaymentID: paymentID, AmountMinor: amount})
	if err != nil {
		// The provider may have executed the request despite a transport
		// failure. Preserve SENT for reconciliation; never reissue Cancel.
		return academy.ErrUnavailable
	}
	// The response alone is not settlement. Leave SENT until the existing
	// signed REFUNDED webhook + GetState writer commits the reversal.
	return nil
}
