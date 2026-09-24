package privacy

import (
	"testing"
	"time"
)

func TestDeleteAccountRequestUsesSameCanonicalLifecycleAcrossSurfaces(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for _, source := range []DeletionSource{DeletionSourceWeb, DeletionSourceIOS, DeletionSourceAndroid} {
		request, err := NewDeleteAccountRequest("request-"+string(source), "identity-1", source, now)
		if err != nil {
			t.Fatalf("%s create: %v", source, err)
		}
		if request.State != DeletionRequested {
			t.Fatalf("%s initial state = %s", source, request.State)
		}
		if err := request.ReconfirmIdentity(now.Add(time.Minute)); err != nil {
			t.Fatalf("%s reconfirm: %v", source, err)
		}
		if err := request.ClassifyRetention([]RetentionItem{
			{DataClass: "PROFILE", Disposition: RetentionErase},
			{DataClass: "FINANCIAL_EVIDENCE", Disposition: RetentionRetain, Reason: "jurisdiction retention policy"},
		}, now.Add(2*time.Minute)); err != nil {
			t.Fatalf("%s classify: %v", source, err)
		}
		if err := request.BeginProviderErasure([]ProviderErasureJob{
			{ProviderRef: "object-storage", State: ErasurePending},
		}, now.Add(3*time.Minute)); err != nil {
			t.Fatalf("%s begin erasure: %v", source, err)
		}
		if err := request.MarkProviderErased("object-storage", "evidence:erased", now.Add(4*time.Minute)); err != nil {
			t.Fatalf("%s provider erasure: %v", source, err)
		}
		if err := request.Complete(now.Add(5 * time.Minute)); err != nil {
			t.Fatalf("%s complete: %v", source, err)
		}
		if request.State != DeletionPartiallyRetainedReason {
			t.Fatalf("%s terminal state = %s", source, request.State)
		}
	}
}

func TestDeletionNeverTreatsDeactivationAsDeletionAndRequiresRetentionReason(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	request, err := NewDeleteAccountRequest("request-1", "identity-1", DeletionSourceWeb, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ReconfirmIdentity(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	err = request.ClassifyRetention([]RetentionItem{
		{DataClass: "AUDIT", Disposition: RetentionRetain},
	}, now.Add(2*time.Minute))
	if err != ErrRetentionReasonRequired {
		t.Fatalf("retention without reason err = %v", err)
	}

	if _, err := NewDeleteAccountRequest("request-2", "identity-1", DeletionSource("DEACTIVATED"), now); err != ErrInvalidDeletionRequest {
		t.Fatalf("deactivation must not be accepted as deletion source: %v", err)
	}
}

func TestDeletionCannotCompleteBeforeProviderEvidence(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	request, _ := NewDeleteAccountRequest("request-1", "identity-1", DeletionSourceAndroid, now)
	_ = request.ReconfirmIdentity(now.Add(time.Minute))
	_ = request.ClassifyRetention([]RetentionItem{
		{DataClass: "PROFILE", Disposition: RetentionErase},
	}, now.Add(2*time.Minute))
	_ = request.BeginProviderErasure([]ProviderErasureJob{
		{ProviderRef: "processor-1", State: ErasurePending},
	}, now.Add(3*time.Minute))

	if err := request.Complete(now.Add(4 * time.Minute)); err != ErrErasureIncomplete {
		t.Fatalf("completion without provider evidence err = %v", err)
	}
}

func TestLegalHoldProducesExplicitWaitingState(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	request, _ := NewDeleteAccountRequest("request-1", "identity-1", DeletionSourceIOS, now)
	_ = request.ReconfirmIdentity(now.Add(time.Minute))
	_ = request.ClassifyRetention([]RetentionItem{
		{
			DataClass:    "SECURITY_LOG",
			Disposition:  RetentionRetain,
			Reason:       "active legal hold",
			LegalHoldRef: "legal-hold-1",
		},
	}, now.Add(2*time.Minute))
	_ = request.BeginProviderErasure(nil, now.Add(3*time.Minute))
	if err := request.Complete(now.Add(4 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if request.State != DeletionWaitingLegalHoldExpiry {
		t.Fatalf("state = %s", request.State)
	}
}
