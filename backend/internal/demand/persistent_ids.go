package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"

func newJourneyID() (string, error) {
	return persistentid.New()
}

func bookingIDForHold(holdID string) (string, error) {
	return persistentid.FromRef("booking-for-hold", holdID)
}
