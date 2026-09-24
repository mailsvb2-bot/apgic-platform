package refunds

import (
	"errors"
	"testing"
	"time"
)

func validRequest(t0 time.Time) Request {
	return Request{
		ID: "refund-1", BookingID: "booking-1", OrderID: "order-1",
		OriginalPaymentAttemptID: "payment-1",
		OriginalProviderID: "provider-a", ProviderID: "provider-a",
		AmountMinor: 10000, Currency: "rub",
		PolicyVersion: "refund-policy-v1", PolicyDecision: "ALLOW",
		ReasonCode: "CLIENT_CANCEL_ALLOWED", IdempotencyKey: "refund-key-1",
		ExecutionOwner: ExternalExecutionOwner, CreatedAt: t0,
	}
}

func TestRefundRequiresOriginalExternalProvider(t *testing.T) {
	t0 := time.Now().UTC()
	request := validRequest(t0)
	request.ProviderID = "provider-b"
	if _, err := New(request); !errors.Is(err, ErrInvalidRefund) {
		t.Fatalf("cross-provider refund must be rejected, got %v", err)
	}

	request = validRequest(t0)
	request.ExecutionOwner = "APGIC"
	if _, err := New(request); !errors.Is(err, ErrInvalidRefund) {
		t.Fatalf("APGIC-owned refund execution must be rejected, got %v", err)
	}
}

func TestRefundLifecycleRequiresProviderEvidence(t *testing.T) {
	t0 := time.Now().UTC()
	request, err := New(validRequest(t0))
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Transition(StateSent, t0.Add(time.Second), "provider/refund-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := request.Transition(StateSucceeded, t0.Add(2*time.Second), "", ""); !errors.Is(err, ErrTransitionDenied) {
		t.Fatalf("success without provider evidence must fail, got %v", err)
	}
	if err := request.Transition(StateSucceeded, t0.Add(3*time.Second), "", "provider-evidence/refund-1"); err != nil {
		t.Fatal(err)
	}
}

func TestAmbiguousRefundCannotBeReSent(t *testing.T) {
	t0 := time.Now().UTC()
	request, err := New(validRequest(t0))
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Transition(StateSent, t0.Add(time.Second), "provider/refund-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := request.Transition(StateAmbiguous, t0.Add(2*time.Second), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := request.Transition(StateSent, t0.Add(3*time.Second), "provider/refund-2", ""); !errors.Is(err, ErrTransitionDenied) {
		t.Fatalf("ambiguous refund must not be blindly re-sent, got %v", err)
	}
}

func TestProviderValidationRejectsAlternativeProvider(t *testing.T) {
	request, err := New(validRequest(time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateExecutionProvider("provider-b"); !errors.Is(err, ErrProviderMismatch) {
		t.Fatalf("expected original-provider guard, got %v", err)
	}
}
