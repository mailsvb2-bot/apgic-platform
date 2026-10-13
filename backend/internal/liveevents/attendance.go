package liveevents

// AttendanceSummary distinguishes provider transport JOINED from actually
// ready realtime media. A waiting-room connection is never an attendee.
type AttendanceSummary struct {
	EventID                string `json:"event_id"`
	Registered             int    `json:"registered"`
	JoinedParticipants     int    `json:"joined_participants"`
	ProviderJoinFacts      int    `json:"provider_join_facts"`
	ProviderLeaveFacts     int    `json:"provider_leave_facts"`
	MediaReadyObservations int    `json:"media_ready_observations"`
	TotalConnectedSeconds  int64  `json:"total_connected_seconds"`
	VerifiedEvidence       bool   `json:"verified_evidence"`
}
