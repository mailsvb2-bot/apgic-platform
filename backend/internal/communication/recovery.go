package communication

import (
	"errors"
	"strings"
	"time"
)

type FailureKind string

const (
	FailureProviderDisconnect FailureKind = "PROVIDER_DISCONNECT"
	FailureRoomFailure        FailureKind = "ROOM_FAILURE"
	FailureNetworkLoss        FailureKind = "NETWORK_LOSS"
)

type RecoveryAction string

const (
	RecoveryRetrySameProvider RecoveryAction = "RETRY_SAME_PROVIDER"
	RecoveryFallbackProvider  RecoveryAction = "FALLBACK_PROVIDER"
	RecoveryReschedule        RecoveryAction = "RESCHEDULE"
	RecoveryRefund            RecoveryAction = "REFUND"
)

var ErrInvalidRecoveryInput = errors.New("invalid communication recovery input")

type TechnicalFailure struct {
	ConsultationID     string
	BookingID          string
	ProviderInstanceID string
	Kind               FailureKind
	Attempt            int
	EvidenceRef        string
	OccurredAt         time.Time
}

type RecoveryPolicy struct {
	Version             string
	MaxRecoveryAttempts int
	AllowFallback       bool
	ExhaustedAction     RecoveryAction
}

type RecoveryContext struct {
	CurrentProviderID   string
	FallbackProviderID  string
	RefundEligible      bool
	RescheduleAvailable bool
}

type RecoveryDecision struct {
	Action              RecoveryAction
	PolicyVersion       string
	ProviderInstanceID  string
	ReasonCode          string
	RefundPathRequired  bool
	RescheduleRequired  bool
}

func DecideRecovery(
	failure TechnicalFailure,
	policy RecoveryPolicy,
	ctx RecoveryContext,
) (RecoveryDecision, error) {
	if strings.TrimSpace(failure.ConsultationID) == "" ||
		strings.TrimSpace(failure.BookingID) == "" ||
		strings.TrimSpace(failure.ProviderInstanceID) == "" ||
		strings.TrimSpace(failure.EvidenceRef) == "" ||
		failure.OccurredAt.IsZero() ||
		failure.Attempt < 0 ||
		strings.TrimSpace(policy.Version) == "" ||
		policy.MaxRecoveryAttempts < 0 ||
		ctx.CurrentProviderID != failure.ProviderInstanceID {
		return RecoveryDecision{}, ErrInvalidRecoveryInput
	}
	switch failure.Kind {
	case FailureProviderDisconnect, FailureRoomFailure, FailureNetworkLoss:
	default:
		return RecoveryDecision{}, ErrInvalidRecoveryInput
	}
	if policy.ExhaustedAction != RecoveryReschedule && policy.ExhaustedAction != RecoveryRefund {
		return RecoveryDecision{}, ErrInvalidRecoveryInput
	}

	decision := RecoveryDecision{PolicyVersion: policy.Version}
	if failure.Attempt < policy.MaxRecoveryAttempts {
		if policy.AllowFallback &&
			strings.TrimSpace(ctx.FallbackProviderID) != "" &&
			ctx.FallbackProviderID != ctx.CurrentProviderID {
			decision.Action = RecoveryFallbackProvider
			decision.ProviderInstanceID = ctx.FallbackProviderID
			decision.ReasonCode = "COMM_RECOVERY_FALLBACK"
			return decision, nil
		}
		decision.Action = RecoveryRetrySameProvider
		decision.ProviderInstanceID = ctx.CurrentProviderID
		decision.ReasonCode = "COMM_RECOVERY_RETRY"
		return decision, nil
	}

	if policy.ExhaustedAction == RecoveryRefund && ctx.RefundEligible {
		decision.Action = RecoveryRefund
		decision.ReasonCode = "COMM_RECOVERY_EXHAUSTED_REFUND"
		decision.RefundPathRequired = true
		return decision, nil
	}
	if ctx.RescheduleAvailable {
		decision.Action = RecoveryReschedule
		decision.ReasonCode = "COMM_RECOVERY_EXHAUSTED_RESCHEDULE"
		decision.RescheduleRequired = true
		return decision, nil
	}
	if ctx.RefundEligible {
		decision.Action = RecoveryRefund
		decision.ReasonCode = "COMM_RECOVERY_EXHAUSTED_REFUND"
		decision.RefundPathRequired = true
		return decision, nil
	}
	return RecoveryDecision{}, ErrInvalidRecoveryInput
}
