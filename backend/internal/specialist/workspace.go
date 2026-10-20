package specialist

import (
	"context"
	"time"
)

// WorkspaceEntry contains only owner-scoped booking and consultation state.
// No private client identity or evidence appears in this projection.
type WorkspaceEntry struct {
	SlotID            string    `json:"slot_id"`
	BookingID         string    `json:"booking_id,omitempty"`
	BookingState      string    `json:"booking_state,omitempty"`
	ConsultationState string    `json:"consultation_state,omitempty"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	TopicID           string    `json:"topic_id,omitempty"`
}

type WorkspaceStore interface {
	SpecialistWorkspace(context.Context, string, time.Time, time.Time) ([]WorkspaceEntry, error)
}
