package finance

import (
	"errors"
	"math"
	"strings"
)

const ExternalExecutionOwner = "EXTERNAL_PROVIDER"

const (
	CapabilityMarketplaceSplit   = "MARKETPLACE_SPLIT"
	CapabilitySettlementExecution = "SETTLEMENT_EXECUTION"
	CapabilityPayoutExecution     = "PAYOUT_EXECUTION"
)

var (
	ErrInvalidCommissionPolicy       = errors.New("invalid commission policy")
	ErrCommissionPolicyNotConfigured = errors.New("commission policy not configured")
	ErrInvalidSettlement             = errors.New("invalid settlement context")
	ErrProviderAffinity              = errors.New("financial lifecycle must use original provider")
	ErrProviderCapability            = errors.New("provider execution capability missing")
	ErrPayoutBlocked                 = errors.New("provider payout eligibility is not confirmed")
)

type CommissionRule struct {
	PolicyVersion           string
	DemandSource            string
	ProductRef              string
	Jurisdiction            string
	BasisPoints             int64
	RoundingMode            string
	PlatformFeeRecipientRef string
}

type CommissionContext struct {
	OrderID      string
	DemandSource string
	ProductRef   string
	Jurisdiction string
	AmountMinor  int64
	Currency     string
}

type CommissionSnapshot struct {
	OrderID                 string
	PolicyVersion           string
	DemandSource            string
	ProductRef              string
	Jurisdiction            string
	BasisPoints             int64
	RoundingMode            string
	BasisAmountMinor        int64
	CommissionMinor         int64
	Currency                string
	PlatformFeeRecipientRef string
}

func CalculateCommission(ctx CommissionContext, rules []CommissionRule) (CommissionSnapshot, error) {
	ctx.Currency = strings.ToUpper(strings.TrimSpace(ctx.Currency))
	if strings.TrimSpace(ctx.OrderID) == "" ||
		strings.TrimSpace(ctx.DemandSource) == "" ||
		strings.TrimSpace(ctx.ProductRef) == "" ||
		strings.TrimSpace(ctx.Jurisdiction) == "" ||
		ctx.AmountMinor <= 0 ||
		len(ctx.Currency) != 3 {
		return CommissionSnapshot{}, ErrInvalidCommissionPolicy
	}

	var matched *CommissionRule
	for i := range rules {
		rule := rules[i]
		if err := validateCommissionRule(rule); err != nil {
			return CommissionSnapshot{}, err
		}
		if rule.DemandSource == ctx.DemandSource &&
			rule.ProductRef == ctx.ProductRef &&
			rule.Jurisdiction == ctx.Jurisdiction {
			if matched != nil {
				return CommissionSnapshot{}, ErrInvalidCommissionPolicy
			}
			copy := rule
			matched = &copy
		}
	}
	if matched == nil {
		return CommissionSnapshot{}, ErrCommissionPolicyNotConfigured
	}
	if matched.BasisPoints != 0 && ctx.AmountMinor > math.MaxInt64/matched.BasisPoints {
		return CommissionSnapshot{}, ErrInvalidCommissionPolicy
	}
	commission := ctx.AmountMinor * matched.BasisPoints / 10000
	return CommissionSnapshot{
		OrderID: ctx.OrderID,
		PolicyVersion: matched.PolicyVersion,
		DemandSource: ctx.DemandSource,
		ProductRef: ctx.ProductRef,
		Jurisdiction: ctx.Jurisdiction,
		BasisPoints: matched.BasisPoints,
		RoundingMode: matched.RoundingMode,
		BasisAmountMinor: ctx.AmountMinor,
		CommissionMinor: commission,
		Currency: ctx.Currency,
		PlatformFeeRecipientRef: matched.PlatformFeeRecipientRef,
	}, nil
}

func validateCommissionRule(rule CommissionRule) error {
	if strings.TrimSpace(rule.PolicyVersion) == "" ||
		strings.TrimSpace(rule.DemandSource) == "" ||
		strings.TrimSpace(rule.ProductRef) == "" ||
		strings.TrimSpace(rule.Jurisdiction) == "" ||
		rule.BasisPoints < 0 ||
		rule.BasisPoints > 10000 ||
		rule.RoundingMode != "FLOOR_MINOR" ||
		strings.TrimSpace(rule.PlatformFeeRecipientRef) == "" {
		return ErrInvalidCommissionPolicy
	}
	return nil
}

type ProviderExecutionProfile struct {
	ProviderID     string
	ExecutionOwner string
	Capabilities   []string
}

func (p ProviderExecutionProfile) Has(capability string) bool {
	for _, value := range p.Capabilities {
		if value == capability {
			return true
		}
	}
	return false
}

type SettlementContext struct {
	InstructionID                string
	IdempotencyKey               string
	OrderID                      string
	PaymentAttemptID             string
	OriginalProviderID           string
	AmountMinor                  int64
	Currency                     string
	CaptureLedgerEntryRef        string
	CaptureProviderEvidenceRef   string
	CompletionEvidenceRef        string
	PayoutBeneficiaryRef         string
	Commission                   CommissionSnapshot
}

type Allocation struct {
	BeneficiaryRef string
	AmountMinor    int64
	Kind           string
}

type SettlementInstruction struct {
	InstructionID      string
	IdempotencyKey     string
	OrderID            string
	PaymentAttemptID   string
	ProviderID         string
	ExecutionOwner     string
	Currency           string
	TotalAmountMinor   int64
	PolicyVersion      string
	LedgerEvidenceRef  string
	ProviderEvidenceRef string
	Allocations        []Allocation
}

func BuildSettlementInstruction(
	ctx SettlementContext,
	provider ProviderExecutionProfile,
) (SettlementInstruction, error) {
	ctx.Currency = strings.ToUpper(strings.TrimSpace(ctx.Currency))
	if strings.TrimSpace(ctx.InstructionID) == "" ||
		strings.TrimSpace(ctx.IdempotencyKey) == "" ||
		strings.TrimSpace(ctx.OrderID) == "" ||
		strings.TrimSpace(ctx.PaymentAttemptID) == "" ||
		strings.TrimSpace(ctx.OriginalProviderID) == "" ||
		ctx.AmountMinor <= 0 ||
		len(ctx.Currency) != 3 ||
		strings.TrimSpace(ctx.CaptureLedgerEntryRef) == "" ||
		strings.TrimSpace(ctx.CaptureProviderEvidenceRef) == "" ||
		strings.TrimSpace(ctx.CompletionEvidenceRef) == "" ||
		strings.TrimSpace(ctx.PayoutBeneficiaryRef) == "" ||
		ctx.Commission.OrderID != ctx.OrderID ||
		ctx.Commission.BasisAmountMinor != ctx.AmountMinor ||
		ctx.Commission.Currency != ctx.Currency ||
		ctx.Commission.CommissionMinor < 0 ||
		ctx.Commission.CommissionMinor > ctx.AmountMinor {
		return SettlementInstruction{}, ErrInvalidSettlement
	}
	if provider.ProviderID != ctx.OriginalProviderID {
		return SettlementInstruction{}, ErrProviderAffinity
	}
	if provider.ExecutionOwner != ExternalExecutionOwner ||
		!provider.Has(CapabilityMarketplaceSplit) ||
		!provider.Has(CapabilitySettlementExecution) {
		return SettlementInstruction{}, ErrProviderCapability
	}

	allocations := []Allocation{{
		BeneficiaryRef: ctx.PayoutBeneficiaryRef,
		AmountMinor: ctx.AmountMinor - ctx.Commission.CommissionMinor,
		Kind: "SERVICE_BENEFICIARY",
	}}
	if ctx.Commission.CommissionMinor > 0 {
		allocations = append(allocations, Allocation{
			BeneficiaryRef: ctx.Commission.PlatformFeeRecipientRef,
			AmountMinor: ctx.Commission.CommissionMinor,
			Kind: "PLATFORM_FEE",
		})
	}

	return SettlementInstruction{
		InstructionID: ctx.InstructionID,
		IdempotencyKey: ctx.IdempotencyKey,
		OrderID: ctx.OrderID,
		PaymentAttemptID: ctx.PaymentAttemptID,
		ProviderID: provider.ProviderID,
		ExecutionOwner: ExternalExecutionOwner,
		Currency: ctx.Currency,
		TotalAmountMinor: ctx.AmountMinor,
		PolicyVersion: ctx.Commission.PolicyVersion,
		LedgerEvidenceRef: ctx.CaptureLedgerEntryRef,
		ProviderEvidenceRef: ctx.CaptureProviderEvidenceRef,
		Allocations: allocations,
	}, nil
}

type SettlementEvidence struct {
	SettlementInstructionID string
	ProviderID               string
	ProviderSettlementRef    string
	ProviderEvidenceRef      string
	LedgerEvidenceRef        string
}

type PayoutEligibility struct {
	DecisionID               string
	SettlementInstructionID  string
	OriginalProviderID       string
	BeneficiaryRef           string
	AmountMinor              int64
	Currency                 string
	ProviderReportedEligible bool
	ComplianceEvidenceRef    string
	IdempotencyKey           string
}

type PayoutInstruction struct {
	DecisionID        string
	SettlementInstructionID string
	ProviderID        string
	ExecutionOwner    string
	BeneficiaryRef    string
	AmountMinor       int64
	Currency          string
	IdempotencyKey    string
	ComplianceEvidenceRef string
}

func BuildPayoutInstruction(
	eligibility PayoutEligibility,
	evidence SettlementEvidence,
	provider ProviderExecutionProfile,
) (PayoutInstruction, error) {
	eligibility.Currency = strings.ToUpper(strings.TrimSpace(eligibility.Currency))
	if strings.TrimSpace(eligibility.DecisionID) == "" ||
		strings.TrimSpace(eligibility.SettlementInstructionID) == "" ||
		strings.TrimSpace(eligibility.OriginalProviderID) == "" ||
		strings.TrimSpace(eligibility.BeneficiaryRef) == "" ||
		eligibility.AmountMinor <= 0 ||
		len(eligibility.Currency) != 3 ||
		strings.TrimSpace(eligibility.IdempotencyKey) == "" ||
		evidence.SettlementInstructionID != eligibility.SettlementInstructionID ||
		strings.TrimSpace(evidence.ProviderSettlementRef) == "" ||
		strings.TrimSpace(evidence.ProviderEvidenceRef) == "" ||
		strings.TrimSpace(evidence.LedgerEvidenceRef) == "" {
		return PayoutInstruction{}, ErrInvalidSettlement
	}
	if provider.ProviderID != eligibility.OriginalProviderID ||
		evidence.ProviderID != eligibility.OriginalProviderID {
		return PayoutInstruction{}, ErrProviderAffinity
	}
	if provider.ExecutionOwner != ExternalExecutionOwner ||
		!provider.Has(CapabilityPayoutExecution) {
		return PayoutInstruction{}, ErrProviderCapability
	}
	if !eligibility.ProviderReportedEligible ||
		strings.TrimSpace(eligibility.ComplianceEvidenceRef) == "" {
		return PayoutInstruction{}, ErrPayoutBlocked
	}

	return PayoutInstruction{
		DecisionID: eligibility.DecisionID,
		SettlementInstructionID: eligibility.SettlementInstructionID,
		ProviderID: provider.ProviderID,
		ExecutionOwner: ExternalExecutionOwner,
		BeneficiaryRef: eligibility.BeneficiaryRef,
		AmountMinor: eligibility.AmountMinor,
		Currency: eligibility.Currency,
		IdempotencyKey: eligibility.IdempotencyKey,
		ComplianceEvidenceRef: eligibility.ComplianceEvidenceRef,
	}, nil
}
