package academy

import (
	"errors"
	"testing"
)

func TestCanSubmitAssessmentAttempt(t *testing.T) {
	if err := CanSubmitAssessmentAttempt(false); err != nil {
		t.Fatalf("eligible new attempt denied: %v", err)
	}
	if err := CanSubmitAssessmentAttempt(true); !errors.Is(err, ErrState) {
		t.Fatalf("pending or passed attempt must block resubmission: %v", err)
	}
}
