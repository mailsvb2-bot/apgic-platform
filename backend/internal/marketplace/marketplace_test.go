package marketplace

import "testing"

func verifiedProfile(t *testing.T, id, topic string) SpecialistProfile {
	t.Helper()
	profile, err := NewSpecialistProfile(id, "identity-"+id, "Specialist "+id)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := NewCapability(topic, EvidenceAPGICVerified, "evidence:"+id)
	if err != nil {
		t.Fatal(err)
	}
	profile.AddCapability(capability)
	profile.Profession = "PSYCHOLOGIST"
	profile.Complete = true
	profile.Review = ReviewApproved
	return *profile
}

func policyFor(topic string) QualificationPolicy {
	return QualificationPolicy{
		Version: "qualification-v1",
		Rules: []QualificationRule{{
			Profession:      "PSYCHOLOGIST",
			Topic:           topic,
			Jurisdiction:    "RU",
			Format:          "ONLINE",
			AgeGroup:        "ADULT",
			MinimumEvidence: EvidenceAPGICVerified,
			Decision:        EligibilityEligible,
		}},
	}
}

func TestCapabilityEvidenceDoesNotPretendToBeVerified(t *testing.T) {
	capability, err := NewCapability("anxiety", EvidenceDocumentSupported, "doc:1")
	if err != nil {
		t.Fatal(err)
	}
	if capability.Verified() {
		t.Fatal("DOCUMENT_SUPPORTED capability must not be marked verified")
	}
}

func TestQualificationFailsClosedAndRequiresEvidence(t *testing.T) {
	profile := verifiedProfile(t, "s1", "anxiety")
	request := QualificationRequest{Topic: "anxiety", Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}

	missingPolicy := QualificationPolicy{}.Evaluate(profile, request)
	if missingPolicy.Decision != EligibilityIneligible || missingPolicy.ReasonCodes[0] != ReasonPolicyNotConfigured {
		t.Fatalf("missing policy result = %#v", missingPolicy)
	}

	noRule := (QualificationPolicy{Version: "q-v1"}).Evaluate(profile, request)
	if noRule.Decision != EligibilityIneligible || noRule.ReasonCodes[0] != ReasonRuleMissing {
		t.Fatalf("missing rule result = %#v", noRule)
	}

	capability, _ := NewCapability("anxiety", EvidenceDocumentSupported, "doc:2")
	profile.Capabilities = []Capability{capability}
	needsReview := policyFor("anxiety").Evaluate(profile, request)
	if needsReview.Decision != EligibilityManualReview || needsReview.ReasonCodes[0] != ReasonEvidenceReview {
		t.Fatalf("insufficient evidence result = %#v", needsReview)
	}
}

func TestQualificationExcludesCommercialBoost(t *testing.T) {
	profile := verifiedProfile(t, "s1", "anxiety")
	request := QualificationRequest{Topic: "anxiety", Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}
	policy := policyFor("anxiety")

	plain := Candidate{Profile: profile, CommercialBoostTier: 0}
	sponsored := Candidate{Profile: profile, CommercialBoostTier: 999}
	plainResult := policy.Evaluate(plain.Profile, request)
	sponsoredResult := policy.Evaluate(sponsored.Profile, request)
	if plainResult.Decision != sponsoredResult.Decision || plainResult.PolicyVersion != sponsoredResult.PolicyVersion {
		t.Fatalf("commercial boost changed qualification: plain=%#v sponsored=%#v", plainResult, sponsoredResult)
	}
}

func TestSupplyMinPublishGate(t *testing.T) {
	profile := verifiedProfile(t, "s1", "anxiety")
	profile.Complete = false
	context := PublishContext{Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}
	policy := policyFor("anxiety")

	blocked := Publish(&profile, policy, context)
	if blocked.Allowed || blocked.ReasonCodes[0] != ReasonProfileIncomplete {
		t.Fatalf("incomplete profile publish = %#v", blocked)
	}

	profile.Complete = true
	profile.Review = ReviewPending
	blocked = Publish(&profile, policy, context)
	if blocked.Allowed || blocked.ReasonCodes[0] != ReasonReviewIncomplete {
		t.Fatalf("pending review publish = %#v", blocked)
	}

	profile.Review = ReviewApproved
	allowed := Publish(&profile, policy, context)
	if !allowed.Allowed || !profile.IsPublishedFor("anxiety") {
		t.Fatalf("approved profile not published: decision=%#v profile=%#v", allowed, profile)
	}

	Unpublish(&profile)
	if profile.PublishState != PublishUnpublished || profile.IsPublishedFor("anxiety") {
		t.Fatalf("unpublish failed: %#v", profile)
	}
}

func TestPublishRevalidationRemovesStaleDiscoverability(t *testing.T) {
	topic := "anxiety"
	profile := verifiedProfile(t, "s1", topic)
	policy := policyFor(topic)
	context := PublishContext{Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}

	if !Publish(&profile, policy, context).Allowed || !profile.IsPublishedFor(topic) {
		t.Fatalf("initial publish failed: %#v", profile)
	}

	profile.Review = ReviewPending
	blocked := Publish(&profile, policy, context)
	if blocked.Allowed || blocked.ReasonCodes[0] != ReasonReviewIncomplete {
		t.Fatalf("review revalidation result = %#v", blocked)
	}
	if profile.PublishState != PublishUnpublished || profile.IsPublishedFor(topic) || len(profile.PublishedTopics) != 0 {
		t.Fatalf("stale published profile survived review revocation: %#v", profile)
	}

	profile.Review = ReviewApproved
	if !Publish(&profile, policy, context).Allowed {
		t.Fatal("republish after review approval failed")
	}
	downgraded, err := NewCapability(topic, EvidenceDocumentSupported, "doc:downgraded")
	if err != nil {
		t.Fatal(err)
	}
	profile.AddCapability(downgraded)
	blocked = Publish(&profile, policy, context)
	if blocked.Allowed || blocked.ReasonCodes[0] != ReasonNoEligibleTopic {
		t.Fatalf("evidence revalidation result = %#v", blocked)
	}
	if profile.PublishState != PublishUnpublished || profile.IsPublishedFor(topic) || len(profile.PublishedTopics) != 0 {
		t.Fatalf("stale published profile survived evidence downgrade: %#v", profile)
	}

	projection := RebuildSearchProjection("search-v2", []SpecialistProfile{profile})
	if got := projection.SearchTopic(topic); len(got) != 0 {
		t.Fatalf("rebuild retained ineligible profile: %#v", got)
	}
}

func TestHelpIntentIsUserCorrectableAndCannotAssertDiagnosis(t *testing.T) {
	_, err := NewHelpIntent("h1", "Мне тревожно перед выступлениями", HelpIntentInterpretation{
		Topics:         []string{"anxiety"},
		DiagnosisClaim: "social_anxiety_disorder",
	})
	if err != ErrDiagnosisAssertionForbidden {
		t.Fatalf("diagnosis assertion err = %v", err)
	}

	intent, err := NewHelpIntent("h1", "Мне тревожно перед выступлениями", HelpIntentInterpretation{
		Topics: []string{"anxiety"},
		Goals:  []string{"public-speaking"},
	})
	if err != nil {
		t.Fatal(err)
	}
	intent.Confirm([]string{"performance-anxiety"}, []string{"public-speaking"}, map[string]string{"format": "online"})
	if intent.Status != HelpIntentConfirmed || intent.Topics[0] != "performance-anxiety" {
		t.Fatalf("intent correction not preserved: %#v", intent)
	}
}

func TestMatchingNeverReturnsIneligibleCandidate(t *testing.T) {
	topic := "anxiety"
	policy := policyFor(topic)
	context := PublishContext{Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}

	eligible := verifiedProfile(t, "eligible", topic)
	if !Publish(&eligible, policy, context).Allowed {
		t.Fatal("eligible profile failed publish")
	}

	ineligible := verifiedProfile(t, "ineligible", topic)
	capability, _ := NewCapability(topic, EvidenceDocumentSupported, "doc:pending")
	ineligible.Capabilities = []Capability{capability}
	ineligible.PublishState = PublishPublished
	ineligible.PublishedTopics = []string{topic}

	intent, _ := NewHelpIntent("h1", "нужна помощь с тревогой", HelpIntentInterpretation{Topics: []string{topic}})
	intent.Confirm([]string{topic}, nil, nil)
	matches, err := MatchEligible(policy, MatchRequest{
		Intent:        *intent,
		Topic:         topic,
		Jurisdiction:  "RU",
		Format:        "ONLINE",
		AgeGroup:      "ADULT",
		Language:      "ru",
		MaxPriceMinor: 500000,
	}, []Candidate{
		{Profile: ineligible, Formats: []string{"ONLINE"}, Languages: []string{"ru"}, PriceMinor: 100000, Available: true},
		{Profile: eligible, Formats: []string{"ONLINE"}, Languages: []string{"ru"}, PriceMinor: 100000, Available: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].SpecialistID != "eligible" {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestRankingIsDeterministicAndSponsoredCannotBypassProtectedBand(t *testing.T) {
	matches := []Match{
		{SpecialistID: "sponsored-low", RelevanceTier: 2, ExpertiseTier: 5, ReliabilityTier: 5, ReputationTier: 5, AvailabilityResponseTier: 5, PlatformExperienceTier: 5, CommercialBoostTier: 100},
		{SpecialistID: "organic-high", RelevanceTier: 3, ExpertiseTier: 1, ReliabilityTier: 1, ReputationTier: 1, AvailabilityResponseTier: 1, PlatformExperienceTier: 1, CommercialBoostTier: 0},
		{SpecialistID: "sponsored-peer", RelevanceTier: 3, ExpertiseTier: 1, ReliabilityTier: 1, ReputationTier: 1, AvailabilityResponseTier: 1, PlatformExperienceTier: 1, CommercialBoostTier: 1},
	}
	ranked := Rank(RankingPolicy{Version: "ranking-v1"}, matches)
	if len(ranked) != 3 {
		t.Fatalf("ranked len = %d", len(ranked))
	}
	if ranked[2].SpecialistID != "sponsored-low" {
		t.Fatalf("lower relevance sponsored candidate bypassed protected band: %#v", ranked)
	}
	if ranked[0].SpecialistID != "sponsored-peer" || !ranked[0].Sponsored {
		t.Fatalf("peer sponsored ordering/label incorrect: %#v", ranked)
	}
	secondRun := Rank(RankingPolicy{Version: "ranking-v1"}, matches)
	for i := range ranked {
		if ranked[i].SpecialistID != secondRun[i].SpecialistID {
			t.Fatalf("ranking not deterministic: %#v vs %#v", ranked, secondRun)
		}
		if ranked[i].RankingPolicyVersion != "ranking-v1" {
			t.Fatalf("ranking policy version missing: %#v", ranked[i])
		}
	}
}

func TestSearchProjectionRebuildsFromCanonicalPublishedProfiles(t *testing.T) {
	profile := verifiedProfile(t, "s1", "anxiety")
	policy := policyFor("anxiety")
	if !Publish(&profile, policy, PublishContext{Jurisdiction: "RU", Format: "ONLINE", AgeGroup: "ADULT"}).Allowed {
		t.Fatal("profile failed publish")
	}

	projection := RebuildSearchProjection("search-v1", []SpecialistProfile{profile})
	entries := projection.SearchTopic("anxiety")
	if len(entries) != 1 || entries[0].SpecialistID != "s1" {
		t.Fatalf("search entries = %#v", entries)
	}

	entries[0].Topics[0] = "tampered"
	if !profile.IsPublishedFor("anxiety") {
		t.Fatal("projection mutation changed canonical publish truth")
	}
	rebuilt := RebuildSearchProjection("search-v2", []SpecialistProfile{profile})
	if got := rebuilt.SearchTopic("anxiety"); len(got) != 1 {
		t.Fatalf("rebuild did not recover canonical projection: %#v", got)
	}
}
