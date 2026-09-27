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
