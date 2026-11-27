package runtimepostgres

import (
	"context"
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyBankCaptureReferenceRejectsForeignAndMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		ref, terminal, payment string
		valid                  bool
	}{
		{"tbank/merchant/123456", "merchant", "123456", true},
		{"tbank/test/000012", "test", "000012", true},
		{"tbank/merchant/", "", "", false},
		{"other/merchant/123", "", "", false},
		{"tbank/merchant/123/extra", "", "", false},
		{"tbank/merchant/not-number", "", "", false},
		{"tbank//123", "", "", false},
		{"tbank/merchant/123\n", "", "", false},
	} {
		terminal, payment, valid := academyParseTBankReference(tc.ref)
		if terminal != tc.terminal || payment != tc.payment || valid != tc.valid {
			t.Errorf("bank reference %q yielded %q/%q %t", tc.ref, terminal, payment, valid)
		}
	}
}

func TestAcademyBankCaptureMustValidateBeforeDatabaseAndBank(t *testing.T) {
	var absent *Checker
	for _, tc := range []struct {
		order string
		raw   []byte
	}{
		{"", []byte("{}")},
		{"foreign", []byte("{}")},
		{"00000000-0000-0000-0000-000000000001", nil},
	} {
		_, err := absent.AcademyCommitBankCapture(context.Background(), tc.order, tc.raw, nil)
		if !errors.Is(err, academy.ErrInvalid) {
			t.Fatalf("invalid bank capture reached database or bank: %v", err)
		}
	}
}
