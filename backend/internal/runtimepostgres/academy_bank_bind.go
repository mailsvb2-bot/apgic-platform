package runtimepostgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyBindBankInit records the unique external PaymentId after a successful
// bank Init response. It does not issue access or treat NEW as a capture.
// A caller may reveal the PSP payment URL only after this commit succeeded.
// In case of timeout/uncertain commit the caller must reconcile this attempt,
// not initiate another bank payment.
func (c *Checker) AcademyBindBankInit(buyer, orderID, attemptID, terminalKey, paymentID string) error {
	if !academyMediaUUID(buyer) || !academyMediaUUID(orderID) || !academyMediaUUID(attemptID) ||
		terminalKey == "" || len(terminalKey) > 128 || strings.ContainsAny(terminalKey, "/ \r\n") ||
		paymentID == "" || len(paymentID) > 32 {
		return academy.ErrInvalid
	}
	for _, digit := range paymentID {
		if digit < '0' || digit > '9' {
			return academy.ErrInvalid
		}
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, providerKind string
	var previous sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT pa.state,pa.provider_reference,ci.provider_kind
 FROM payment_attempts pa
 JOIN orders o ON o.id=pa.order_id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 JOIN academy_bank_seller_bindings seller ON seller.product_owner_ref=o.product_owner_ref
  AND seller.provider_config_id=cfg.id
  AND seller.credential_version_ref=cfg.credential_version_ref
  AND seller.revoked_at IS NULL
  AND seller.terminal_key=$4
 WHERE o.id=$1::uuid AND pa.id=$2::uuid AND o.direct_buyer_identity_id=$3::uuid
  AND o.booking_id IS NULL AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER'
  AND route.selected_method_code='BANK_CARD'
 FOR UPDATE OF pa`, orderID, attemptID, buyer, terminalKey).Scan(&state, &previous, &providerKind)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrDenied
	}
	if err != nil {
		return err
	}
	if providerKind != "TBANK" {
		return academy.ErrDenied
	}
	ref := "tbank/" + terminalKey + "/" + paymentID
	if previous.Valid {
		if previous.String != ref || (state != "SENT" && state != "PENDING" && state != "AMBIGUOUS" && state != "SUCCEEDED") {
			return academy.ErrConflict
		}
		return tx.Commit()
	}
	if state != "SENT" {
		return academy.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE payment_attempts
 SET provider_reference=$1,state='SENT',updated_at=now()
 WHERE id=$2::uuid AND order_id=$3::uuid AND state='SENT' AND provider_reference IS NULL`,
		ref, attemptID, orderID)
	if err != nil {
		return fmt.Errorf("bind bank payment to canonical attempt: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return academy.ErrConflict
	}
	return tx.Commit()
}
