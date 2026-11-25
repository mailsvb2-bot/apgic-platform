package runtimepostgres

import (
	"context"
	"encoding/json"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyBankCommitNotification is the existing Commerce capture writer's
// narrow bank-HTTP adapter. A signed notification AND fresh bank GetState
// must succeed before one atomic ledger/entitlement transaction can commit.
func (c *Checker) AcademyBankCommitNotification(ctx context.Context, orderID string,
	notification []byte, bank *payments.TBankAcquiring) error {
	// Status only selects the proof path; neither branch trusts this field
	// until the signed notification and independent bank state agree.
	var envelope struct {
		Status string `json:"Status"`
	}
	if json.Unmarshal(notification, &envelope) != nil {
		return academy.ErrInvalid
	}
	if envelope.Status == "REFUNDED" {
		return c.AcademyCommitBankRefund(ctx, orderID, notification, bank)
	}
	_, err := c.AcademyCommitBankCapture(ctx, orderID, notification, bank)
	return err
}
