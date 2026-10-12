package clientpersonal

import (
	"errors"
	"time"
)

var ErrCheckInInvalid = errors.New("invalid client check-in")
var ErrCheckInNotEligible = errors.New("provider-completed owned consultation required")
var ErrCheckInConflict = errors.New("immutable client report already exists")

type CheckInInput struct {
	BookingID         string `json:"booking_id"`
	ProgressCode      string `json:"progress_code"`
	MatchSatisfaction *int   `json:"match_satisfaction,omitempty"`
}
type CheckIn struct {
	CheckInInput
	RecordedAt time.Time `json:"recorded_at"`
	Idempotent bool      `json:"idempotent"`
}

func ValidateCheckIn(v CheckInInput) error {
	if len(v.BookingID) != 36 || !uuidLike(v.BookingID) {
		return ErrCheckInInvalid
	}
	switch v.ProgressCode {
	case "IMPROVED", "UNCHANGED", "WORSE", "UNSURE":
	default:
		return ErrCheckInInvalid
	}
	if v.MatchSatisfaction != nil && (*v.MatchSatisfaction < 1 || *v.MatchSatisfaction > 5) {
		return ErrCheckInInvalid
	}
	return nil
}
