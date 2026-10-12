package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

type liveSupplyFixture struct {
	offers []BookableOffer
	err    error
}

func (s *liveSupplyFixture) PublishedProfiles() ([]marketplace.SpecialistProfile, error) {
	return nil, s.err
}

func (s *liveSupplyFixture) BookableOffers() ([]BookableOffer, error) {
	return s.offers, s.err
}

func publishedLiveProfile(t *testing.T, topic string) marketplace.SpecialistProfile {
	t.Helper()
	profile, err := marketplace.NewSpecialistProfile(
		"5b7e4811-9abc-4e27-a490-cf85f2267601",
		"9ef8d120-8f88-4c28-a5f6-7f03cd48dd01", "Реальный специалист",
	)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := marketplace.NewCapability(topic, marketplace.EvidenceAPGICVerified, "reviewed-document:123")
	if err != nil {
		t.Fatal(err)
	}
	profile.AddCapability(capability)
	profile.Complete = true
	profile.Review = marketplace.ReviewApproved
	profile.Profession = "PSYCHOLOGIST"
	profile.PublishState = marketplace.PublishPublished
	profile.PublishedTopics = []string{topic}
	return *profile
}

func liveFixtureOffer(t *testing.T, topic, offerID string, price int64, start time.Time) BookableOffer {
	t.Helper()
	profile := publishedLiveProfile(t, topic)
	return BookableOffer{
		ID: offerID, Profile: profile, Topic: topic, PriceMinor: price, Currency: "RUB",
		Slots: []Slot{{
			ID:           profile.ID + "-slot-" + start.Format("20060102T1504Z"),
			SpecialistID: profile.ID, OfferID: offerID, Topic: topic,
			StartsAt: start, EndsAt: start.Add(50 * time.Minute), Exclusive: true,
		}},
	}
}

func TestLiveSupplyDoesNotInventPriceOrSlots(t *testing.T) {
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	good := liveFixtureOffer(t, "anxiety", "11111111-1111-4111-8111-111111111111", 185000, now.Add(time.Hour))
	invalidPrice := liveFixtureOffer(t, "sleep", "22222222-2222-4222-8222-222222222222", 0, now.Add(2*time.Hour))
	invalidSlot := liveFixtureOffer(t, "sleep", "33333333-3333-4333-8333-333333333333", 250000, now.Add(3*time.Hour))
	invalidSlot.Slots[0].OfferID = "different"
	unpublished := liveFixtureOffer(t, "anxiety", "44444444-4444-4444-8444-444444444444", 250000, now.Add(4*time.Hour))
	unpublished.Profile.PublishState = marketplace.PublishUnpublished
	c := liveCatalogFromOffers([]BookableOffer{good, invalidPrice, invalidSlot, unpublished})
	if len(c.candidates) != 1 || len(c.slots) != 1 ||
		c.candidates[0].OfferID != good.ID || c.candidates[0].PriceMinor != 185000 {
		t.Fatalf("unverified or unpriced offer appeared as bookable: candidates=%#v slots=%#v", c.candidates, c.slots)
	}
	if c.slots[0].OfferID != good.ID {
		t.Fatalf("slot lost canonical offer identity: %#v", c.slots[0])
	}
}

func TestLiveSupplyMatchesSelectedPricedOfferAndHoldsOnlyConfirmedTopic(t *testing.T) {
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	anxiety := liveFixtureOffer(t, "anxiety", "11111111-1111-4111-8111-111111111111", 185000, now.Add(2*time.Hour))
	sleep := liveFixtureOffer(t, "sleep", "22222222-2222-4222-8222-222222222222", 290000, now.Add(3*time.Hour))
	service := NewConformanceService(func() time.Time { return now })
	service.liveSupplyMode = true
	service.specialistStore = &liveSupplyFixture{offers: []BookableOffer{anxiety, sleep}}
	service.catalog = liveCatalogFromOffers([]BookableOffer{anxiety, sleep})
	intent, err := service.CreateIntentForIdentity("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "мне тревожно")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"anxiety"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	cards, _, err := service.Matches(intent.ID, "anxiety")
	if err != nil || len(cards) != 1 || cards[0].OfferID != anxiety.ID || cards[0].PriceMinor != 185000 {
		t.Fatalf("live matches = %#v, err=%v", cards, err)
	}
	// One verified specialist can offer multiple reviewed topics at different
	// real prices. The exact offer identity, not the specialist, owns pricing.
	sleepIntent, err := service.CreateIntentForIdentity("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "трудно спать")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(sleepIntent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	sleepMatches, _, err := service.Matches(sleepIntent.ID, "sleep")
	if err != nil || len(sleepMatches) != 1 || sleepMatches[0].OfferID != sleep.ID || sleepMatches[0].PriceMinor != 290000 {
		t.Fatalf("multiple services were cross-priced: %#v err=%v", sleepMatches, err)
	}
	slots, err := service.SlotsForOffer(anxiety.Profile.ID, cards[0].OfferID)
	if err != nil || len(slots) != 1 || slots[0].ID != anxiety.Slots[0].ID {
		t.Fatalf("live slot selection = %#v, err=%v", slots, err)
	}
	if _, err := service.AcquireHold(intent.ID, sleep.Slots[0].ID, intent.ClientIdentityID); !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("unconfirmed priced topic acquired hold: %v", err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil || hold.BookingState != "HELD" {
		t.Fatalf("valid priced slot was not held: %#v, err=%v", hold, err)
	}
}

func TestVerifiedOutcomeTiersNeverInventReputation(t *testing.T) {
	cases := []struct {
		completed, noShow       int64
		reliability, experience int
	}{
		{0, 0, 0, 0}, {1, 0, 0, 1}, {4, 0, 0, 1},
		{5, 0, 5, 3}, {5, 5, 2, 3}, {20, 0, 5, 4},
		{100, 0, 5, 5}, {-1, 0, 0, 0},
	}
	for _, tc := range cases {
		reliability, experience := verifiedOutcomeTiers(tc.completed, tc.noShow)
		if reliability != tc.reliability || experience != tc.experience {
			t.Fatalf("outcomes (%d,%d): reliability=%d experience=%d", tc.completed, tc.noShow, reliability, experience)
		}
	}
}

func TestVerifiedFeedbackRankingRequiresReviewedBookingSample(t *testing.T) {
	cases := []struct {
		count, sum int64
		want int
	}{
		{0, 0, 0}, {4, 20, 0}, {5, 25, 4}, {20, 100, 5},
		{10, 30, 2}, {5, 5, 1}, {5, 4, 0},
	}
	for _, item := range cases {
		got := verifiedFeedbackTier(item.count, item.sum)
		if got != item.want {
			t.Fatalf("verified rating count=%d sum=%d: got %d want %d", item.count, item.sum, got, item.want)
		}
	}
	offer := liveFixtureOffer(t, "anxiety", "aaaa1111-1111-4111-8111-111111111111",
		210000, time.Date(2026, 12, 14, 14, 0, 0, 0, time.UTC))
	offer.VerifiedReviewCount = 20
	offer.VerifiedRatingTotal = 100
	ranked := liveCatalogFromOffers([]BookableOffer{offer})
	if len(ranked.candidates) != 1 || ranked.candidates[0].ReputationTier != 5 ||
		ranked.candidates[0].CommercialBoostTier != 0 {
		t.Fatalf("verified booking feedback was not trusted or became sponsored: %#v", ranked.candidates)
	}
	offer.VerifiedReviewCount = 4
	offer.VerifiedRatingTotal = 20
	few := liveCatalogFromOffers([]BookableOffer{offer})
	if len(few.candidates) != 1 || few.candidates[0].ReputationTier != 0 {
		t.Fatalf("too few private feedback records became an invented rating: %#v", few.candidates)
	}
}
