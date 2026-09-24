package communication

import (
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type ParticipantRole string

const (
	RoleClient     ParticipantRole = "CLIENT"
	RoleSpecialist ParticipantRole = "SPECIALIST"
)

type Decision string

const (
	Allow Decision = "ALLOW"
	Deny  Decision = "DENY"
)

const (
	ReasonAllowed             = "COMM_JOIN_ALLOWED"
	ReasonBookingNotJoinable  = "COMM_BOOKING_NOT_JOINABLE"
	ReasonRoleMismatch        = "COMM_ROLE_MISMATCH"
	ReasonEntitlementMissing  = "COMM_ENTITLEMENT_REQUIRED"
	ReasonOutsideJoinWindow   = "COMM_OUTSIDE_JOIN_WINDOW"
	ReasonProviderUnavailable = "COMM_PROVIDER_UNAVAILABLE"
	ReasonInvalidJoinRequest  = "COMM_JOIN_REQUEST_INVALID"
	ReasonInvalidJoinPolicy   = "COMM_JOIN_POLICY_INVALID"
)

var (
	ErrInvalidJoinRequest = errors.New("invalid communication join request")
	ErrInvalidJoinPolicy  = errors.New("invalid communication join policy")
)

type BookingAccess struct {
	BookingID            string
	State                booking.State
	ClientIdentityID     string
	SpecialistIdentityID string
}

type EntitlementProof struct {
	ID          string
	BookingID   string
	IdentityID  string
	Active      bool
	EvidenceRef string
	ExpiresAt   time.Time
}

type JoinWindow struct {
	OpensAt  time.Time
	ClosesAt time.Time
}

type JoinPolicy struct {
	Version          string
	MaxCredentialTTL time.Duration
}

type JoinRequest struct {
	IdentityID     string
	Role           ParticipantRole
	IdempotencyKey string
	Now            time.Time
}

type CredentialRequest struct {
	ProviderInstanceID string
	BookingID          string
	IdentityID         string
	Role               ParticipantRole
	Scope              string
	IdempotencyKey     string
	ExpiresAt          time.Time
}

type JoinDecision struct {
	Decision           Decision
	ReasonCode         string
	PolicyVersion      string
	ProviderInstanceID string
	CredentialRequest  *CredentialRequest
}

func AuthorizeJoin(
	request JoinRequest,
	access BookingAccess,
	entitlement EntitlementProof,
	window JoinWindow,
	provider connector.Instance,
	policy JoinPolicy,
) (JoinDecision, error) {
	base := JoinDecision{Decision: Deny, PolicyVersion: policy.Version}
	if strings.TrimSpace(request.IdentityID) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" ||
		request.Now.IsZero() ||
		(request.Role != RoleClient && request.Role != RoleSpecialist) {
		base.ReasonCode = ReasonInvalidJoinRequest
		return base, ErrInvalidJoinRequest
	}
	if strings.TrimSpace(policy.Version) == "" ||
		policy.MaxCredentialTTL <= 0 ||
		window.OpensAt.IsZero() ||
		window.ClosesAt.IsZero() ||
		!window.ClosesAt.After(window.OpensAt) {
		base.ReasonCode = ReasonInvalidJoinPolicy
		return base, ErrInvalidJoinPolicy
	}
	if access.State != booking.StateConfirmed {
		base.ReasonCode = ReasonBookingNotJoinable
		return base, nil
	}
	if !roleMatches(request, access) {
		base.ReasonCode = ReasonRoleMismatch
		return base, nil
	}
	if entitlement.ID == "" ||
		entitlement.BookingID != access.BookingID ||
		entitlement.IdentityID != request.IdentityID ||
		!entitlement.Active ||
		strings.TrimSpace(entitlement.EvidenceRef) == "" ||
		!entitlement.ExpiresAt.After(request.Now) {
		base.ReasonCode = ReasonEntitlementMissing
		return base, nil
	}
	if request.Now.Before(window.OpensAt) || !request.Now.Before(window.ClosesAt) {
		base.ReasonCode = ReasonOutsideJoinWindow
		return base, nil
	}
	if provider.Capability != connector.CapabilityCommunication ||
		(provider.Status != connector.StatusActive && provider.Status != connector.StatusDegraded) ||
		strings.TrimSpace(provider.ID) == "" {
		base.ReasonCode = ReasonProviderUnavailable
		return base, nil
	}

	expiresAt := request.Now.Add(policy.MaxCredentialTTL)
	if expiresAt.After(window.ClosesAt) {
		expiresAt = window.ClosesAt
	}

	base.Decision = Allow
	base.ReasonCode = ReasonAllowed
	base.ProviderInstanceID = provider.ID
	base.CredentialRequest = &CredentialRequest{
		ProviderInstanceID: provider.ID,
		BookingID:          access.BookingID,
		IdentityID:         request.IdentityID,
		Role:               request.Role,
		Scope:              "ROOM_JOIN",
		IdempotencyKey:     request.IdempotencyKey,
		ExpiresAt:          expiresAt,
	}
	return base, nil
}

func roleMatches(request JoinRequest, access BookingAccess) bool {
	switch request.Role {
	case RoleClient:
		return request.IdentityID == access.ClientIdentityID
	case RoleSpecialist:
		return request.IdentityID == access.SpecialistIdentityID
	default:
		return false
	}
}
