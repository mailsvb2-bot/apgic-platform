package marketplace

import (
	"errors"
	"sort"
)

var (
	ErrHelpIntentTextRequired      = errors.New("help intent free text is required")
	ErrDiagnosisAssertionForbidden = errors.New("help intent interpretation cannot assert a diagnosis")
	ErrHelpIntentNotConfirmed      = errors.New("help intent must be confirmed before matching")
)

type HelpIntentStatus string

const (
	HelpIntentDraft     HelpIntentStatus = "DRAFT"
	HelpIntentConfirmed HelpIntentStatus = "CONFIRMED"
)

type HelpIntentInterpretation struct {
	Topics         []string
	Goals          []string
	Context        map[string]string
	DiagnosisClaim string
}

type HelpIntent struct {
	ID       string
	FreeText string
	Topics   []string
	Goals    []string
	Context  map[string]string
	Status   HelpIntentStatus
}

func NewHelpIntent(id, freeText string, interpretation HelpIntentInterpretation) (*HelpIntent, error) {
	if freeText == "" {
		return nil, ErrHelpIntentTextRequired
	}
	if interpretation.DiagnosisClaim != "" {
		return nil, ErrDiagnosisAssertionForbidden
	}
	return &HelpIntent{
		ID:       id,
		FreeText: freeText,
		Topics:   cloneStrings(interpretation.Topics),
		Goals:    cloneStrings(interpretation.Goals),
		Context:  cloneMap(interpretation.Context),
		Status:   HelpIntentDraft,
	}, nil
}

func (h *HelpIntent) Confirm(topics, goals []string, context map[string]string) {
	h.Topics = cloneStrings(topics)
	h.Goals = cloneStrings(goals)
	h.Context = cloneMap(context)
	h.Status = HelpIntentConfirmed
}

type PublishContext struct {
	Jurisdiction string
	Format       string
	AgeGroup     string
}

type PublishDecision struct {
	Allowed         bool
	ReasonCodes     []string
	PublishedTopics []string
	PolicyVersion   string
}

const (
	ReasonProfileIncomplete = "PUBLISH_PROFILE_INCOMPLETE"
	ReasonReviewIncomplete  = "PUBLISH_REVIEW_INCOMPLETE"
	ReasonNoCapability      = "PUBLISH_NO_CAPABILITY"
	ReasonNoEligibleTopic   = "PUBLISH_NO_ELIGIBLE_TOPIC"
	ReasonPublished         = "PUBLISH_ALLOWED"
	ReasonUnpublished       = "PUBLISH_UNPUBLISHED"
)

func Publish(profile *SpecialistProfile, policy QualificationPolicy, context PublishContext) PublishDecision {
	decision := PublishDecision{PolicyVersion: policy.Version}
	if !profile.Complete {
		blockPublication(profile)
		decision.ReasonCodes = []string{ReasonProfileIncomplete}
		return decision
	}
	if profile.Review != ReviewApproved {
		blockPublication(profile)
		decision.ReasonCodes = []string{ReasonReviewIncomplete}
		return decision
	}
	if len(profile.Capabilities) == 0 {
		blockPublication(profile)
		decision.ReasonCodes = []string{ReasonNoCapability}
		return decision
	}

	eligibleTopics := make([]string, 0, len(profile.Capabilities))
	for _, capability := range profile.Capabilities {
		result := policy.Evaluate(*profile, QualificationRequest{
			Topic:        capability.Topic,
			Jurisdiction: context.Jurisdiction,
			Format:       context.Format,
			AgeGroup:     context.AgeGroup,
		})
		if result.Decision == EligibilityEligible {
			eligibleTopics = append(eligibleTopics, capability.Topic)
		}
	}
	if len(eligibleTopics) == 0 {
		blockPublication(profile)
		decision.ReasonCodes = []string{ReasonNoEligibleTopic}
		return decision
	}

	sort.Strings(eligibleTopics)
	profile.PublishState = PublishPublished
	profile.PublishedTopics = cloneStrings(eligibleTopics)
	decision.Allowed = true
	decision.ReasonCodes = []string{ReasonPublished}
	decision.PublishedTopics = cloneStrings(eligibleTopics)
	return decision
}

func Unpublish(profile *SpecialistProfile) PublishDecision {
	profile.PublishState = PublishUnpublished
	profile.PublishedTopics = nil
	return PublishDecision{Allowed: true, ReasonCodes: []string{ReasonUnpublished}}
}

func blockPublication(profile *SpecialistProfile) {
	if profile.PublishState == PublishPublished {
		profile.PublishState = PublishUnpublished
	}
	profile.PublishedTopics = nil
}

type Candidate struct {
	Profile                  SpecialistProfile
	Formats                  []string
	Languages                []string
	PriceMinor               int64
	Available                bool
	ExpertiseTier            int
	ReliabilityTier          int
	ReputationTier           int
	AvailabilityResponseTier int
	PlatformExperienceTier   int
	CommercialBoostTier      int
}

type MatchRequest struct {
	Intent        HelpIntent
	Topic         string
	Jurisdiction  string
	Format        string
	AgeGroup      string
	Language      string
	MaxPriceMinor int64
}

type Match struct {
	SpecialistID             string
	DisplayName              string
	RelevanceTier            int
	ExpertiseTier            int
	ReliabilityTier          int
	ReputationTier           int
	AvailabilityResponseTier int
	PlatformExperienceTier   int
	CommercialBoostTier      int
	Qualification            QualificationResult
}

func MatchEligible(policy QualificationPolicy, request MatchRequest, candidates []Candidate) ([]Match, error) {
	if request.Intent.Status != HelpIntentConfirmed {
		return nil, ErrHelpIntentNotConfirmed
	}
	if request.Topic == "" || !containsString(request.Intent.Topics, request.Topic) {
		return []Match{}, nil
	}

	matches := make([]Match, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Profile.IsPublishedFor(request.Topic) {
			continue
		}
		qualification := policy.Evaluate(candidate.Profile, QualificationRequest{
			Topic:        request.Topic,
			Jurisdiction: request.Jurisdiction,
			Format:       request.Format,
			AgeGroup:     request.AgeGroup,
		})
		if qualification.Decision != EligibilityEligible {
			continue
		}
		if request.Format != "" && !containsString(candidate.Formats, request.Format) {
			continue
		}
		if request.Language != "" && !containsString(candidate.Languages, request.Language) {
			continue
		}
		if request.MaxPriceMinor > 0 && candidate.PriceMinor > request.MaxPriceMinor {
			continue
		}
		if !candidate.Available {
			continue
		}
		matches = append(matches, Match{
			SpecialistID:             candidate.Profile.ID,
			DisplayName:              candidate.Profile.DisplayName,
			RelevanceTier:            3,
			ExpertiseTier:            candidate.ExpertiseTier,
			ReliabilityTier:          candidate.ReliabilityTier,
			ReputationTier:           candidate.ReputationTier,
			AvailabilityResponseTier: candidate.AvailabilityResponseTier,
			PlatformExperienceTier:   candidate.PlatformExperienceTier,
			CommercialBoostTier:      candidate.CommercialBoostTier,
			Qualification:            qualification,
		})
	}
	return matches, nil
}

type RankingPolicy struct {
	Version string
}

type RankedMatch struct {
	Match
	RankingPolicyVersion string
	Sponsored            bool
	ReasonCodes          []string
}

const (
	ReasonRankedDeterministically = "RANK_LEXICOGRAPHIC_V1"
	ReasonSponsored               = "RANK_SPONSORED"
)

func Rank(policy RankingPolicy, matches []Match) []RankedMatch {
	copyOf := append([]Match(nil), matches...)
	sort.SliceStable(copyOf, func(i, j int) bool {
		a, b := copyOf[i], copyOf[j]
		keysA := []int{a.RelevanceTier, a.ExpertiseTier, a.ReliabilityTier, a.ReputationTier, a.AvailabilityResponseTier, a.PlatformExperienceTier, a.CommercialBoostTier}
		keysB := []int{b.RelevanceTier, b.ExpertiseTier, b.ReliabilityTier, b.ReputationTier, b.AvailabilityResponseTier, b.PlatformExperienceTier, b.CommercialBoostTier}
		for k := range keysA {
			if keysA[k] != keysB[k] {
				return keysA[k] > keysB[k]
			}
		}
		return a.SpecialistID < b.SpecialistID
	})

	ranked := make([]RankedMatch, 0, len(copyOf))
	for _, match := range copyOf {
		reasons := []string{ReasonRankedDeterministically}
		sponsored := match.CommercialBoostTier > 0
		if sponsored {
			reasons = append(reasons, ReasonSponsored)
		}
		ranked = append(ranked, RankedMatch{
			Match:                match,
			RankingPolicyVersion: policy.Version,
			Sponsored:            sponsored,
			ReasonCodes:          reasons,
		})
	}
	return ranked
}

type SearchEntry struct {
	SpecialistID string
	DisplayName  string
	Topics       []string
}

type SearchProjection struct {
	Version string
	entries []SearchEntry
}

func RebuildSearchProjection(version string, profiles []SpecialistProfile) SearchProjection {
	entries := make([]SearchEntry, 0, len(profiles))
	for _, profile := range profiles {
		if profile.PublishState != PublishPublished || len(profile.PublishedTopics) == 0 {
			continue
		}
		entries = append(entries, SearchEntry{
			SpecialistID: profile.ID,
			DisplayName:  profile.DisplayName,
			Topics:       cloneStrings(profile.PublishedTopics),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SpecialistID < entries[j].SpecialistID })
	return SearchProjection{Version: version, entries: entries}
}

func (p SearchProjection) Drop(specialistID string) SearchProjection {
	kept := make([]SearchEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		if entry.SpecialistID == specialistID {
			continue
		}
		copyEntry := entry
		copyEntry.Topics = cloneStrings(entry.Topics)
		kept = append(kept, copyEntry)
	}
	return SearchProjection{Version: p.Version, entries: kept}
}

func (p SearchProjection) SearchTopic(topic string) []SearchEntry {
	result := make([]SearchEntry, 0)
	for _, entry := range p.entries {
		if containsString(entry.Topics, topic) {
			copyEntry := entry
			copyEntry.Topics = cloneStrings(entry.Topics)
			result = append(result, copyEntry)
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
