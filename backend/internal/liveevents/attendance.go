package liveevents

// AttendanceSummary contains only verified provider observations. It is not a
// participation certificate, a financial entitlement, or a media transcript.
type AttendanceSummary struct {
	EventID             string `json:"event_id"`
	Registered          int    `json:"registered"`
	JoinedParticipants  int    `json:"joined_participants"`
	ProviderJoinFacts   int    `json:"provider_join_facts"`
	ProviderLeaveFacts  int    `json:"provider_leave_facts"`
	VerifiedEvidence    bool   `json:"verified_evidence"`
}
