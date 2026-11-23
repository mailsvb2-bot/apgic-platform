package runtimepostgres

import (
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyBankInitBindingRejectsInvalidIdentityBeforeDB(t *testing.T) {
	var absent *Checker
	valid := "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct{ name, buyer, order, attempt, terminal, payment string }{
		{"missing buyer", "", valid, valid, "merchant", "123"},
		{"bad order", valid, "foreign", valid, "merchant", "123"},
		{"bad attempt", valid, valid, "", "merchant", "123"},
		{"missing terminal", valid, valid, valid, "", "123"},
		{"injection terminal", valid, valid, valid, "merchant/other", "123"},
		{"newline terminal", valid, valid, valid, "merchant\nother", "123"},
		{"missing payment", valid, valid, valid, "merchant", ""},
		{"non numeric payment", valid, valid, valid, "merchant", "123abc"},
		{"oversize payment", valid, valid, valid, "merchant", "123456789012345678901234567890123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := absent.AcademyBindBankInit(tc.buyer, tc.order, tc.attempt, tc.terminal, tc.payment)
			if !errors.Is(err, academy.ErrInvalid) {
				t.Fatalf("accepted unsafe bank init binding: %v", err)
			}
		})
	}
}
