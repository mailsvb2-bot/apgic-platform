package connector

// ConsultationResult is a projection of the durable consultation state.
// The canonical facts are owned by APGIC PostgreSQL, not the media provider.
type ConsultationResult struct {
	BookingID             string `json:"booking_id"`
	State                 string `json:"state"`
	ProviderInstanceID    string `json:"provider_instance_id"`
	CompletionEvidenceRef string `json:"completion_evidence_ref,omitempty"`
}
