package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyBankSellerTerminalAuthorized checks only persistent canonical
// Commerce and the operator-approved immutable seller/terminal binding.
// It has no external PSP or entitlement side effects.
// Call before claiming SENT and before the real T-Bank Init network request.
func (c *Checker) AcademyBankSellerTerminalAuthorized(
	ctx context.Context, buyer, orderID, attemptID, terminalKey string,
) error {
	if ctx == nil || c == nil || c.db == nil ||
		!academyMediaUUID(buyer) || !academyMediaUUID(orderID) ||
		!academyMediaUUID(attemptID) || terminalKey == "" {
		return academy.ErrInvalid
	}
	var enabled bool
	err := c.db.QueryRowContext(ctx, `
 SELECT EXISTS(
  SELECT 1 FROM orders o
  JOIN payment_attempts pa ON pa.order_id=o.id
  JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
  JOIN payment_provider_config_versions cfg ON cfg.id=route.provider_config_id
  JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
  JOIN academy_bank_seller_bindings seller
    ON seller.product_owner_ref=o.product_owner_ref
   AND seller.provider_config_id=cfg.id
   AND seller.credential_version_ref=cfg.credential_version_ref
  WHERE o.id=$1::uuid AND pa.id=$2::uuid
   AND o.direct_buyer_identity_id=$3::uuid
   AND o.booking_id IS NULL AND seller.terminal_key=$4
   AND seller.revoked_at IS NULL AND cfg.status='ACTIVE'
   AND ci.status='ACTIVE' AND ci.capability_class='PAYMENT_PROVIDER'
   AND ci.provider_kind='TBANK'
   AND route.selected_method_code='BANK_CARD'
   AND route.selected_rail_code='APGIC_PAYMENT_PROVIDER'
   AND pa.state='CREATED' AND pa.provider_reference IS NULL
 )`, orderID, attemptID, buyer, terminalKey).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) || !enabled {
		return academy.ErrDenied
	}
	return err
}
