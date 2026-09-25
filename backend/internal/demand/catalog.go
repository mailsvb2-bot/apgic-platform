package demand

import (
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

const (
	CatalogModeConformance = "CONFORMANCE"
	PolicyVersion          = "qualification-v1"
	RankingVersion         = "ranking-v1"
	JurisdictionRU         = "RU"
	FormatOnline           = "ONLINE"
	AgeGroupAdult          = "ADULT"
	LanguageRU             = "ru"
)

type Slot struct {
	ID           string    `json:"id"`
	SpecialistID string    `json:"specialist_id"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Exclusive    bool      `json:"exclusive"`
}

type catalog struct {
	policy     marketplace.QualificationPolicy
	candidates []marketplace.Candidate
	slots      []Slot
	names      map[string]string
}

func conformanceCatalog(now time.Time) catalog {
	policy := marketplace.QualificationPolicy{
		Version: PolicyVersion,
		Rules: []marketplace.QualificationRule{
			rule("PSYCHOLOGIST", "anxiety"),
			rule("PSYCHOLOGIST", "sleep"),
			rule("PSYCHOLOGIST", "relationships"),
			rule("CAREER_COACH", "career"),
		},
	}
	context := marketplace.PublishContext{
		Jurisdiction: JurisdictionRU,
		Format:       FormatOnline,
		AgeGroup:     AgeGroupAdult,
	}

	lebedeva := mustProfile("spec-lebedeva", "Марина Лебедева", "PSYCHOLOGIST",
		cap("anxiety"), cap("sleep"))
	sokolov := mustProfile("spec-sokolov", "Илья Соколов", "PSYCHOLOGIST", cap("anxiety"))
	kim := mustProfile("spec-kim", "Ольга Ким", "CAREER_COACH", cap("career"))
	volkova := mustProfile("spec-volkova", "Анна Волкова", "PSYCHOLOGIST", cap("relationships"))
	draft := mustProfile("spec-draft", "Черновик не публикуется", "PSYCHOLOGIST", cap("anxiety"))
	draft.Complete = false

	published := []*marketplace.SpecialistProfile{&lebedeva, &sokolov, &kim, &volkova}
	for _, profile := range published {
		decision := marketplace.Publish(profile, policy, context)
		if !decision.Allowed {
			panic("conformance catalog failed publish: " + profile.ID)
		}
	}

	day := 26 * time.Hour
	candidates := []marketplace.Candidate{
		candidate(lebedeva, 450000, 5, 5, 4, 0),
		candidate(sokolov, 320000, 2, 3, 3, 1),
		candidate(kim, 500000, 4, 4, 4, 0),
		candidate(volkova, 390000, 4, 4, 5, 0),
		{Profile: draft, Formats: []string{FormatOnline}, Languages: []string{LanguageRU}, PriceMinor: 100000, Available: true},
	}
	slots := make([]Slot, 0, len(published)*8)
	for _, profile := range published {
		for n := 1; n <= 8; n++ {
			start := now.Add(time.Duration(n) * day)
			slots = append(slots, Slot{
				ID:           fmt.Sprintf("%s-slot-%d", profile.ID, n),
				SpecialistID: profile.ID,
				StartsAt:     start,
				EndsAt:       start.Add(50 * time.Minute),
				Exclusive:    true,
			})
		}
	}
	names := map[string]string{}
	for _, profile := range append(published, &draft) {
		names[profile.ID] = profile.DisplayName
	}
	return catalog{policy: policy, candidates: candidates, slots: slots, names: names}
}

func rule(profession, topic string) marketplace.QualificationRule {
	return marketplace.QualificationRule{
		Profession:      profession,
		Topic:           topic,
		Jurisdiction:    JurisdictionRU,
		Format:          FormatOnline,
		AgeGroup:        AgeGroupAdult,
		MinimumEvidence: marketplace.EvidenceAPGICVerified,
		Decision:        marketplace.EligibilityEligible,
	}
}

func cap(topic string) marketplace.Capability {
	capability, err := marketplace.NewCapability(topic, marketplace.EvidenceAPGICVerified, "evidence:"+topic)
	if err != nil {
		panic(err)
	}
	return capability
}

func mustProfile(id, name, profession string, capabilities ...marketplace.Capability) marketplace.SpecialistProfile {
	profile, err := marketplace.NewSpecialistProfile(id, "identity-"+id, name)
	if err != nil {
		panic(err)
	}
	profile.Profession = profession
	profile.Complete = true
	profile.Review = marketplace.ReviewApproved
	for _, capability := range capabilities {
		profile.AddCapability(capability)
	}
	return *profile
}

func candidate(profile marketplace.SpecialistProfile, price int64, expertise, reliability, reputation, boost int) marketplace.Candidate {
	return marketplace.Candidate{
		Profile:                  profile,
		Formats:                  []string{FormatOnline},
		Languages:                []string{LanguageRU},
		PriceMinor:               price,
		Available:                true,
		ExpertiseTier:            expertise,
		ReliabilityTier:          reliability,
		ReputationTier:           reputation,
		AvailabilityResponseTier: 3,
		PlatformExperienceTier:   3,
		CommercialBoostTier:      boost,
	}
}
