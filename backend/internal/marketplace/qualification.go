package marketplace

type Eligibility string

const (
	EligibilityEligible     Eligibility = "ELIGIBLE"
	EligibilityIneligible   Eligibility = "INELIGIBLE"
	EligibilityManualReview Eligibility = "MANUAL_REVIEW"
)

const (
	ReasonPolicyNotConfigured  = "QUAL_POLICY_NOT_CONFIGURED"
	ReasonCapabilityMissing    = "QUAL_CAPABILITY_MISSING"
	ReasonRuleMissing          = "QUAL_EXPLICIT_RULE_MISSING"
	ReasonEvidenceReview       = "QUAL_EVIDENCE_REVIEW_REQUIRED"
	ReasonEvidenceInvalid      = "QUAL_EVIDENCE_INVALID"
	ReasonManualReviewRequired = "QUAL_MANUAL_REVIEW_REQUIRED"
	ReasonEligible             = "QUAL_ELIGIBLE"
)

type QualificationRule struct {
	Profession      string
	Topic           string
	Jurisdiction    string
	Format          string
	AgeGroup        string
	MinimumEvidence EvidenceState
	Decision        Eligibility
}

type QualificationPolicy struct {
	Version string
	Rules   []QualificationRule
}

type QualificationRequest struct {
	Topic        string
	Jurisdiction string
	Format       string
	AgeGroup     string
}

type QualificationResult struct {
	Decision      Eligibility
	PolicyVersion string
	ReasonCodes   []string
	EvidenceRefs  []string
}

func (p QualificationPolicy) Evaluate(profile SpecialistProfile, request QualificationRequest) QualificationResult {
	result := QualificationResult{PolicyVersion: p.Version}
	if p.Version == "" {
		result.Decision = EligibilityIneligible
		result.ReasonCodes = []string{ReasonPolicyNotConfigured}
		return result
	}

	capability, ok := profile.Capability(request.Topic)
	if !ok {
		result.Decision = EligibilityIneligible
		result.ReasonCodes = []string{ReasonCapabilityMissing}
		return result
	}
	result.EvidenceRefs = cloneStrings(capability.EvidenceRefs)

	if evidenceRank(capability.Evidence) == 0 {
		result.Decision = EligibilityIneligible
		result.ReasonCodes = []string{ReasonEvidenceInvalid}
		return result
	}

	rule, ok := p.findRule(profile, request)
	if !ok {
		result.Decision = EligibilityIneligible
		result.ReasonCodes = []string{ReasonRuleMissing}
		return result
	}

	if rule.Decision == EligibilityManualReview {
		result.Decision = EligibilityManualReview
		result.ReasonCodes = []string{ReasonManualReviewRequired}
		return result
	}
	if rule.Decision != EligibilityEligible {
		result.Decision = EligibilityIneligible
		result.ReasonCodes = []string{ReasonRuleMissing}
		return result
	}
	if evidenceRank(capability.Evidence) < evidenceRank(rule.MinimumEvidence) {
		result.Decision = EligibilityManualReview
		result.ReasonCodes = []string{ReasonEvidenceReview}
		return result
	}

	result.Decision = EligibilityEligible
	result.ReasonCodes = []string{ReasonEligible}
	return result
}

func (p QualificationPolicy) findRule(profile SpecialistProfile, request QualificationRequest) (QualificationRule, bool) {
	for _, rule := range p.Rules {
		if rule.Profession == "" || rule.Profession != profile.Profession {
			continue
		}
		if rule.Topic != request.Topic {
			continue
		}
		if rule.Jurisdiction != "" && rule.Jurisdiction != request.Jurisdiction {
			continue
		}
		if rule.Format != "" && rule.Format != request.Format {
			continue
		}
		if rule.AgeGroup != "" && rule.AgeGroup != request.AgeGroup {
			continue
		}
		if evidenceRank(rule.MinimumEvidence) == 0 {
			continue
		}
		return rule, true
	}
	return QualificationRule{}, false
}
