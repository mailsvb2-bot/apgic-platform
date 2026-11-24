package payments

import (
	"context"
)

// ConfirmRefundNotificationWithGetState verifies two distinct bank signals
// against the original persisted Commerce order/payment. The caller must
// independently prove an approved, full, non-replayed canonical refund request
// under a database lock before committing refund receipt/effect/reversal.
// This method has NO rights or ledger side effects.
func (a *TBankAcquiring) ConfirmRefundNotificationWithGetState(
	ctx context.Context, signedNotification []byte, expected TBankCaptureExpectation,
) (TBankCapturedEvent, error) {
	if a == nil || expected.TerminalKey != a.terminalKey {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	webhook, err := VerifyTBankRefundedEvent(signedNotification, a.password, expected)
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	state, err := a.GetRefundedState(ctx, expected)
	if err != nil || state != webhook {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	return webhook, nil
}
