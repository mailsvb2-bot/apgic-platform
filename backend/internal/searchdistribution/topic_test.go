package searchdistribution

import "testing"

func TestReviewedTopicsAreStrictAndStable(t *testing.T) {
	seen := make(map[string]bool)
	for _, slug := range Slugs() {
		topic, ok := TopicForSlug(slug)
		if !ok || topic.Slug != slug || topic.Code == "" || topic.Title == "" || seen[slug] {
			t.Fatalf("invalid or duplicate canonical topic %q", slug)
		}
		seen[slug] = true
	}
	if len(seen) != 4 {
		t.Fatalf("canonical ontology changed without reviewed coverage: %d", len(seen))
	}
	for _, invalid := range []string{"../admin", "TREVoGA", "", "new-topic", "__proto__"} {
		if _, ok := TopicForSlug(invalid); ok {
			t.Fatalf("unreviewed slug accepted: %q", invalid)
		}
	}
}

func TestPublicationGateCountsDistinctAvailablePeople(t *testing.T) {
	first := Offer{SpecialistID: "a", OfferID: "one", PriceMinor: 400000, Currency: "RUB", HasAvailableSlot: true}
	second := Offer{SpecialistID: "b", OfferID: "two", PriceMinor: 500000, Currency: "RUB", HasAvailableSlot: true}
	if Publishable(nil) || Publishable([]Offer{first}) || Publishable([]Offer{first, first}) {
		t.Fatal("thin or duplicate page was indexed")
	}
	if !Publishable([]Offer{first, second}) {
		t.Fatal("real distinct available supply not indexed")
	}
	second.HasAvailableSlot = false
	if Publishable([]Offer{first, second}) {
		t.Fatal("unavailable supply indexed")
	}
	second.HasAvailableSlot = true
	second.PriceMinor = 0
	if Publishable([]Offer{first, second}) {
		t.Fatal("offer without price indexed")
	}
	second.PriceMinor = 500000
	second.Currency = "OTHER"
	if Publishable([]Offer{first, second}) {
		t.Fatal("unverified price currency indexed")
	}
}
