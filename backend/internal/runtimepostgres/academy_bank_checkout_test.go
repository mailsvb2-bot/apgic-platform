package runtimepostgres

import (
	"context"
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyRunBankCheckoutNoPSPWithoutTrustedInputs(t *testing.T) {
	var checker *Checker
	for _, tc := range []struct {
		name        string
		ctx         context.Context
		description string
	}{
		{"no context", nil, "Course"}, {"missing title", context.Background(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checker.AcademyRunBankCheckout(tc.ctx,
				"11111111-1111-4111-8111-111111111111",
				"22222222-2222-4222-8222-222222222222", "safe-checkout-request",
				AcademyBankLegalPolicy{}, nil, tc.description,
				"https://apgic.ru/v1/payments/tbank/notification", nil)
			if !errors.Is(err, academy.ErrInvalid) {
				t.Fatalf("bank checkout accepted invalid context/config: %v", err)
			}
		})
	}
}
