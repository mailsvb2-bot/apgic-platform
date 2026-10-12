package connector

import "time"

// ConsultationTimeline is a bounded read-only business projection.
// It never contains recordings, transcript, clinical notes or media credentials.
type ConsultationTimeline struct {
	BookingID         string                     `json:"booking_id"`
	BookingState      string                     `json:"booking_state"`
	ConsultationState string                     `json:"consultation_state"`
	Facts             []ConsultationTimelineFact `json:"facts"`
	Recovery          []ConsultationRecoveryFact `json:"recovery"`
	RequiresFollowup  bool                       `json:"requires_followup"`
}

type ConsultationTimelineFact struct {
	FactType    string    `json:"fact_type"`
	Role        string    `json:"role"`
	EvidenceRef string    `json:"evidence_ref"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type ConsultationRecoveryFact struct {
	Action          string    `json:"action"`
	ReasonCode      string    `json:"reason_code"`
	FollowupPathRef string    `json:"followup_path_ref,omitempty"`
	DecidedAt       time.Time `json:"decided_at"`
}
