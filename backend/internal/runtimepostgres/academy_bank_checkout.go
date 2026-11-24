package runtimepostgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyBankCheckoutResult is a provider-owned payment redirect, not proof
// of payment. Rights are granted only by AcademyCommitBankCapture.
type AcademyBankCheckoutResult struct {
	OrderID    string
	AttemptID  string
	PaymentID  string
	PaymentURL string
	State      string
}

// AcademyRunBankCheckout uses the one canonical checkout execution path:
// persist legal snapshot/order/route/CREATED attempt -> claim SENT once ->
// external PSP Init -> persist PaymentId -> return bank payment URL.
// policy and receipt MUST originate from an approved server-side seller
// configuration, never browser input or invented tax settings.
// A crashed/ambiguous Init remains SENT without PaymentId; replay never
// sends a second bank charge, and reconciliation is required.
func (c *Checker) AcademyRunBankCheckout(
	ctx context.Context, buyer, courseID, requestID string,
	policy AcademyBankLegalPolicy, receipt json.RawMessage, description string,
	notificationURL string, bank *payments.TBankAcquiring,
) (AcademyBankCheckoutResult, error) {
	if ctx == nil || bank == nil || len(receipt) == 0 ||
		description == "" || notificationURL == "" {
		return AcademyBankCheckoutResult{}, academy.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return AcademyBankCheckoutResult{}, err
	}
	prepared, err := c.AcademyPrepareBankAttempt(buyer, courseID, requestID, policy)
	if err != nil {
		return AcademyBankCheckoutResult{}, err
	}
	// Never regenerate a payment URL by calling Init on a persisted attempt.
	// This covers concurrent HTTP workers, timeouts and process restarts.
	if prepared.Replayed {
		return AcademyBankCheckoutResult{OrderID: prepared.OrderID,
			AttemptID: prepared.AttemptID, State: prepared.State}, academy.ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return AcademyBankCheckoutResult{}, err
	}
	// The Bank instance supplied by the server must be authorized for the
	// exact owner of this persisted Order. This check happens BEFORE the
	// one-shot SENT claim and BEFORE any external PSP call.
	if err = c.AcademyBankSellerTerminalAuthorized(ctx, buyer, prepared.OrderID,
		prepared.AttemptID, bank.TerminalKey()); err != nil {
		return AcademyBankCheckoutResult{}, err
	}
	if err = c.AcademyClaimBankInit(buyer, prepared.OrderID, prepared.AttemptID); err != nil {
		return AcademyBankCheckoutResult{}, err
	}
	result, err := bank.Init(ctx, payments.TBankInitRequest{
		OrderID: prepared.OrderID, AmountMinor: prepared.AmountMinor,
		Description: description, Receipt: receipt, NotificationURL: notificationURL,
	})
	if err != nil {
		// Don't reset SENT to CREATED: bank may already have accepted Init.
		return AcademyBankCheckoutResult{}, errors.Join(academy.ErrUnavailable, err)
	}
	if err = c.AcademyBindBankInit(buyer, prepared.OrderID, prepared.AttemptID,
		bank.TerminalKey(), result.PaymentID); err != nil {
		// PSP response may be valid even when the DB update times out.
		// Never return a payment URL without a committed provider binding.
		return AcademyBankCheckoutResult{}, err
	}
	return AcademyBankCheckoutResult{
		OrderID: prepared.OrderID, AttemptID: prepared.AttemptID,
		PaymentID: result.PaymentID, PaymentURL: result.PaymentURL, State: "SENT",
	}, nil
}
