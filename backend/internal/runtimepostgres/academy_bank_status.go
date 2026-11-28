package runtimepostgres

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type AcademyBankOrderStatus = academy.BankOrderStatus

func (c *Checker) AcademyReadBankOrderStatus(buyer, orderID string) (AcademyBankOrderStatus, error) {
	if !academyMediaUUID(buyer) || !academyMediaUUID(orderID) {
		return AcademyBankOrderStatus{}, academy.ErrInvalid
	}
	return c.academyReadBankOrderStatus(buyer, orderID, "", "")
}

// AcademyRecoverBankOrderStatus resolves a lost checkout response using
// the same immutable buyer/course/idempotency basis as order creation.
func (c *Checker) AcademyRecoverBankOrderStatus(buyer, courseID, key string) (AcademyBankOrderStatus, error) {
	if !academyMediaUUID(buyer) || !academyMediaUUID(courseID) ||
		len(key) < 8 || len(key) > 128 ||
		strings.Trim(key, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._~-") != "" {
		return AcademyBankOrderStatus{}, academy.ErrInvalid
	}
	return c.academyReadBankOrderStatus(buyer, "", courseID, key)
}

func (c *Checker) academyReadBankOrderStatus(buyer, orderID, courseID, key string) (AcademyBankOrderStatus, error) {
	if c == nil || c.db == nil {
		return AcademyBankOrderStatus{}, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var out AcademyBankOrderStatus
	var reference sql.NullString
	var entitlement, refundID, refundState sql.NullString
	var active bool
	err := c.db.QueryRowContext(ctx, `SELECT o.id::text,
 substring(o.offer_ref from 16),o.amount_minor,o.currency,
 pa.state,pa.provider_reference,
 t.id::text,coalesce(t.state='ACTIVE' AND academy_entitlement_is_active(t.id),false),
 refund.id::text,refund.state
 FROM orders o
 JOIN payment_attempts pa ON pa.order_id=o.id
 JOIN payment_routing_decisions r ON r.id=pa.routing_decision_id
 JOIN payment_provider_config_versions cfg ON cfg.id=r.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 LEFT JOIN store_transaction_verifications sv ON sv.payment_attempt_id=pa.id
 LEFT JOIN store_entitlements t ON t.verification_id=sv.id AND t.identity_id=o.direct_buyer_identity_id
 LEFT JOIN LATERAL (
 SELECT rr.id,rr.state FROM refund_requests rr
 WHERE rr.order_id=o.id AND rr.original_payment_attempt_id=pa.id
 AND rr.booking_id IS NULL
 ORDER BY rr.created_at DESC,rr.id DESC LIMIT 1
 ) refund ON true
 WHERE o.direct_buyer_identity_id=$2::uuid
  AND (o.id=NULLIF($1, '')::uuid OR ($1='' AND o.offer_ref='academy/course/'||$3 AND o.direct_checkout_idempotency_key=$4))
  AND o.booking_id IS NULL AND ci.provider_kind='TBANK'
  AND r.selected_rail_code='APGIC_PAYMENT_PROVIDER'
  AND r.selected_method_code='BANK_CARD'
 ORDER BY pa.created_at DESC,pa.id DESC LIMIT 1`, orderID, buyer, courseID, key).
		Scan(&out.OrderID, &out.CourseID, &out.AmountMinor, &out.Currency,
			&out.PaymentState, &reference, &entitlement, &active, &refundID, &refundState)
	if errors.Is(err, sql.ErrNoRows) {
		return AcademyBankOrderStatus{}, academy.ErrNotFound
	}
	if err != nil {
		return AcademyBankOrderStatus{}, err
	}
	out.HasProviderReference = reference.Valid
	out.EntitlementActive = active
	if entitlement.Valid {
		out.EntitlementID = entitlement.String
	}
	if refundID.Valid && refundState.Valid {
		out.RefundID, out.RefundState = refundID.String, refundState.String
	}
	out.RequiresReconciliation = out.RefundState == "SENT" || out.RefundState == "PENDING" ||
		out.RefundState == "AMBIGUOUS" || out.PaymentState == "AMBIGUOUS" ||
		(out.PaymentState == "SENT" && !reference.Valid)
	return out, nil
}
