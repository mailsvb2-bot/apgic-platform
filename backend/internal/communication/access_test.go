package communication

import (
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

func validJoinFixture() (JoinRequest, BookingAccess, EntitlementProof, JoinWindow, connector.Instance, JoinPolicy) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	request := JoinRequest{
		IdentityID: "client-1", Role: RoleClient,
		IdempotencyKey: "join-client-1", Now: now,
	}
	access := BookingAccess{
		BookingID: "booking-1", State: booking.StateConfirmed,
		ClientIdentityID: "client-1", SpecialistIdentityID: "specialist-1",
	}
	entitlement := EntitlementProof{
		ID: "entitlement-1", BookingID: "booking-1", IdentityID: "client-1",
		Active: true, EvidenceRef: "payment/entitlement-1", ExpiresAt: now.Add(time.Hour),
	}
	window := JoinWindow{OpensAt: now.Add(-10 * time.Minute), ClosesAt: now.Add(50 * time.Minute)}
	provider := connector.Instance{
		ID: "communication-provider-1", Capability: connector.CapabilityCommunication,
		ProviderKind: "CI_COMMUNICATION_A", Status: connector.StatusActive,
		ConfigRef: "secret://ci/communication-a",
	}
	policy := JoinPolicy{Version: "communication-access-v1", MaxCredentialTTL: 5 * time.Minute}
	return request, access, entitlement, window, provider, policy
}

func TestJoinAuthorizationProducesOnlyScopedShortLivedCredentialRequest(t *testing.T) {
	request, access, entitlement, window, provider, policy := validJoinFixture()
	decision, err := AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Allow || decision.ReasonCode != ReasonAllowed {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.CredentialRequest == nil {
		t.Fatal("allow must produce provider credential request")
	}
	if decision.CredentialRequest.Scope != "ROOM_JOIN" {
		t.Fatalf("unexpected scope: %s", decision.CredentialRequest.Scope)
	}
	if got := decision.CredentialRequest.ExpiresAt.Sub(request.Now); got != 5*time.Minute {
		t.Fatalf("credential ttl = %s", got)
	}
}

func TestJoinFailsClosedForRoleMismatchOrMissingEntitlement(t *testing.T) {
	request, access, entitlement, window, provider, policy := validJoinFixture()

	request.IdentityID = "other-client"
	decision, err := AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Deny || decision.ReasonCode != ReasonRoleMismatch {
		t.Fatalf("role mismatch decision = %#v", decision)
	}

	request, access, entitlement, window, provider, policy = validJoinFixture()
	entitlement.Active = false
	decision, err = AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Deny || decision.ReasonCode != ReasonEntitlementMissing {
		t.Fatalf("missing entitlement decision = %#v", decision)
	}
}

func TestJoinFailsClosedOutsideWindowOrWithWrongProviderCapability(t *testing.T) {
	request, access, entitlement, window, provider, policy := validJoinFixture()
	request.Now = window.OpensAt.Add(-time.Second)

	decision, err := AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Deny || decision.ReasonCode != ReasonOutsideJoinWindow {
		t.Fatalf("outside-window decision = %#v", decision)
	}

	request, access, entitlement, window, provider, policy = validJoinFixture()
	provider.Capability = connector.CapabilityNotification
	decision, err = AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Deny || decision.ReasonCode != ReasonProviderUnavailable {
		t.Fatalf("wrong-provider decision = %#v", decision)
	}
}

func TestJoinRequiresConfirmedBooking(t *testing.T) {
	request, access, entitlement, window, provider, policy := validJoinFixture()
	access.State = booking.StateCancelled
	decision, err := AuthorizeJoin(request, access, entitlement, window, provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Deny || decision.ReasonCode != ReasonBookingNotJoinable {
		t.Fatalf("cancelled booking decision = %#v", decision)
	}
}
