package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyClaimBankInit atomically reserves the ONE external Init call for the
// persisted canonical attempt. Caller MUST commit this SENT transition before
// doing any network I/O. A replay is never allowed to reissue Init, including
// after a timeout, crash, or an uncertain provider response.
// A missing PaymentId remains explicitly SENT/unknown for reconciliation.
func (c *Checker) AcademyClaimBankInit(buyer, orderID, attemptID string) error {
	if !academyMediaUUID(buyer) || !academyMediaUUID(orderID) || !academyMediaUUID(attemptID) {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	var prior sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT pa.state,pa.provider_reference
 FROM payment_attempts pa JOIN orders o ON o.id=pa.order_id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 WHERE pa.id=$1::uuid AND o.id=$2::uuid
  AND o.direct_buyer_identity_id=$3::uuid AND o.booking_id IS NULL
  AND ci.provider_kind='TBANK'
  AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER'
  AND route.selected_method_code='BANK_CARD'
 FOR UPDATE OF pa`, attemptID, orderID, buyer).Scan(&state, &prior)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrDenied
	}
	if err != nil {
		return err
	}
	if state != "CREATED" || prior.Valid {
		return academy.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE payment_attempts
 SET state='SENT',updated_at=now()
 WHERE id=$1::uuid AND order_id=$2::uuid AND state='CREATED'
 AND provider_reference IS NULL`, attemptID, orderID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return academy.ErrConflict
	}
	return tx.Commit()
}
