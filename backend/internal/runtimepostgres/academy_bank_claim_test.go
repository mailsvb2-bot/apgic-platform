package runtimepostgres

import (
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyBankInitClaimRejectsMalformedIdentityBeforeDatabase(t *testing.T) {
	var nilStore *Checker
	good := "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct{ buyer, order, attempt string }{
		{"", good, good}, {good, "", good}, {good, good, ""},
		{"external", good, good}, {good, "../../wrong", good},
	} {
		if err := nilStore.AcademyClaimBankInit(tc.buyer, tc.order, tc.attempt); !errors.Is(err, academy.ErrInvalid) {
			t.Fatalf("invalid pre-init claim accepted: %+v err=%v", tc, err)
		}
	}
}
