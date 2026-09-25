package demand

import (
	"errors"
	"testing"
)

func TestGrowthCannotReceiveRawConsultationWithoutPurposeConsent(t *testing.T) {
	service, bookingID := confirmedSession(t)
	done, err := service.CompleteSession(bookingID, "provider-room-end")
	if err != nil || done.RawContentStored || done.State != "COMPLETED" {
		t.Fatalf("complete = %#v err=%v", done, err)
	}
	if _, err := service.ExportSessionToGrowth(bookingID, false); !errors.Is(err, ErrPurposeConsent) {
		t.Fatalf("export without consent err = %v", err)
	}
	exported, err := service.ExportSessionToGrowth(bookingID, true)
	if err != nil || !exported.Allowed || exported.RawContentIncluded || exported.State != "COMPLETED" {
		t.Fatalf("consented export = %#v err=%v", exported, err)
	}
}
