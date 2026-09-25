package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"

type GrowthExport struct {
	Allowed            bool   `json:"allowed"`
	Classification     string `json:"classification"`
	PurposeConsent     bool   `json:"purpose_consent"`
	RawContentIncluded bool   `json:"raw_content_included"`
	State              string `json:"state"`
	Notice             string `json:"notice"`
}

func (s *Service) ExportSessionToGrowth(bookingID string, purposeConsent bool) (*GrowthExport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionLocked(bookingID)
	if err != nil {
		return nil, err
	}
	if err := privacy.CanExportToGrowth(privacy.RawConsultation, purposeConsent); err != nil {
		return nil, ErrPurposeConsent
	}
	return &GrowthExport{
		Allowed:            true,
		Classification:     string(privacy.RawConsultation),
		PurposeConsent:     true,
		RawContentIncluded: false,
		State:              string(session.State),
		Notice:             "Отдельное согласие есть, но сырой записи у APGIC нет. Наружу уходит только состояние консультации.",
	}, nil
}
