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
	// Durable booking lifecycle outcomes, not self-reported reputation.
	CompletedConsultations int64
	NoShowConsultations    int64
	// Moderated ratings tied to actual completed client-owned bookings.
	VerifiedReviewCount int64
	VerifiedRatingTotal int64
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
		validSlots := make([]Slot, 0, len(offer.Slots))
		for _, slot := range offer.Slots {
			if !slot.Exclusive || slot.ID == "" || slot.OfferID != offer.ID ||
				slot.SpecialistID != offer.Profile.ID || slot.Topic != offer.Topic {
				continue
			}
			validSlots = append(validSlots, slot)
		}
		if len(validSlots) == 0 {
			continue
		}
		// Trusted completed/no-show outcomes provide a conservative sample;
		// no review score or paid sponsorship is ever inferred.
		reliability, experience := verifiedOutcomeTiers(offer.CompletedConsultations, offer.NoShowConsultations)
		reputation := verifiedFeedbackTier(offer.VerifiedReviewCount, offer.VerifiedRatingTotal)
		availability := len(validSlots)
		if availability > 5 {
			availability = 5
		}
		result.candidates = append(result.candidates, marketplace.Candidate{
			OfferID: offer.ID, Profile: offer.Profile, Formats: []string{FormatOnline},
			Languages: []string{LanguageRU}, PriceMinor: offer.PriceMinor,
			Available: true, ExpertiseTier: 1,
			ReliabilityTier: reliability, PlatformExperienceTier: experience,
			AvailabilityResponseTier: availability, ReputationTier: reputation,
			// CommercialBoostTier remains zero pending trusted promotion owner.
		})
		result.names[offer.Profile.ID] = offer.Profile.DisplayName
		result.slots = append(result.slots, validSlots...)
	}
	return result
}

func verifiedOutcomeTiers(completed, noShow int64) (reliability, experience int) {
	if completed < 0 || noShow < 0 {
		return 0, 0
	}
	switch {
	case completed >= 100:
		experience = 5
	case completed >= 20:
		experience = 4
	case completed >= 5:
		experience = 3
	case completed >= 1:
		experience = 1
	}
	if completed+noShow < 5 {
		return 0, experience
	}
	return int((5 * completed) / (completed + noShow)), experience
}

func verifiedFeedbackTier(count, total int64) int {
	// No public score for small samples. Conservative Bayesian shrinkage
	// towards a neutral three-star prior prevents few high ratings from
	// dominating experienced professionals.
	if count < 5 || count > 1000000000 || total < count || total > 5*count {
		return 0
	}
	adjusted := float64(total+15) / float64(count+5)
	switch {
	case adjusted >= 4.5:
		return 5
	case adjusted >= 4:
		return 4
	case adjusted >= 3.5:
		return 3
	case adjusted >= 3:
		return 2
	default:
		return 1
	}
}
