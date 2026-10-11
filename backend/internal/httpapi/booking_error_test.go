package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestDemandFailureMapsLiveBookingConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/slot-holds", nil)

	writeDemandFailure(recorder, request, demand.ErrSlotBooked)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusConflict)
	}
	if !contains(recorder.Body.String(), "BOOK_SLOT_BOOKED") {
		t.Fatalf("stable booking conflict code missing: %s", recorder.Body.String())
	}
}

func TestLiveProviderUnavailableFailsClosedWithStablePaymentError(t *testing.T) {
	for _, path := range []string{"/v1/slot-holds/hold-fixture/checkout-options", "/v1/checkout"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, nil)
		writeDemandFailure(recorder, request, demand.ErrLivePaymentUnavailable)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status=%d want 503", path, recorder.Code)
		}
		if !contains(recorder.Body.String(), "PAY_REAL_PROVIDER_REQUIRED") {
			t.Fatalf("%s: stable production-only provider error absent: %s", path, recorder.Body.String())
		}
		if contains(recorder.Body.String(), "external-bank") {
			t.Fatalf("%s: conformance bank leaked into production error: %s", path, recorder.Body.String())
		}
	}
}
