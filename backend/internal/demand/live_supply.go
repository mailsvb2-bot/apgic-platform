package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"

// BookableOffer is an explicitly priced, PostgreSQL-owned consultation offer.
// A profile publication alone cannot become a purchasable MatchCard.
// The offer must have real exclusive future slots to be returned here.
type BookableOffer struct {
	ID         string
	Profile    marketplace.SpecialistProfile
	Topic      string
	PriceMinor int64
	Currency   string
	Slots      []Slot
}

// LiveBookableStore is only a projection of the canonical specialist/Product/
// booking-slot owners in PostgreSQL. The conformance fixture is never used
// to synthesize an offer or a price in live mode.
type LiveBookableStore interface {
	BookableOffers() ([]BookableOffer, error)
}

func liveCatalogFromOffers(offers []BookableOffer) catalog {
	result := liveCatalog()
	result.candidates = make([]marketplace.Candidate, 0, len(offers))
	result.slots = make([]Slot, 0)
	for _, offer := range offers {
		if offer.ID == "" || offer.PriceMinor <= 0 || offer.Currency != "RUB" ||
			!offer.Profile.IsPublishedFor(offer.Topic) || len(offer.Slots) == 0 {
			continue
		}
		// One versioned price per published offer/topic; no inferred score or
		// sponsored placement until a policy-controlled score exists.
		result.candidates = append(result.candidates, marketplace.Candidate{
			Profile: offer.Profile, Formats: []string{FormatOnline},
			Languages: []string{LanguageRU}, PriceMinor: offer.PriceMinor,
			Available: true,
		})
		result.names[offer.Profile.ID] = offer.Profile.DisplayName
		for _, slot := range offer.Slots {
			if !slot.Exclusive || slot.ID == "" || slot.OfferID != offer.ID ||
				slot.SpecialistID != offer.Profile.ID || slot.Topic != offer.Topic {
				continue
			}
			result.slots = append(result.slots, slot)
		}
	}
	return result
}
