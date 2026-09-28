package runtimepostgres

import (
	"context"
	"os"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestSpecialistSupplyPersistsEvidenceReviewAndPublishGate(t *testing.T) {
	databaseURL := os.Getenv("APGIC_SPECIALIST_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("specialist integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	identityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}

	profile, err := store.UpsertProfile(identityID, "Тестовый Специалист", "PSYCHOLOGIST")
	if err != nil {
		t.Fatal(err)
	}
	if profile.IdentityID != identityID || !profile.ProfileComplete || profile.ReviewState != marketplace.ReviewPending {
		t.Fatalf("initial specialist profile = %#v", profile)
	}

	roles, _, err := store.IdentityRoles(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsIdentityRole(roles, identity.RoleSpecialist) {
		t.Fatalf("specialist role not attached to canonical identity: %v", roles)
	}

	profile, err = store.DeclareCapability(identityID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Capabilities) != 1 {
		t.Fatalf("capabilities = %#v", profile.Capabilities)
	}
	capability := profile.Capabilities[0]
	if capability.EvidenceState != marketplace.EvidenceSelfDeclared || capability.VerificationState != "NOT_APPLICABLE" {
		t.Fatalf("self-declared capability overstated verification: %#v", capability)
	}

	blocked, err := store.Publish(identityID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Allowed || len(blocked.ReasonCodes) != 1 || blocked.ReasonCodes[0] != marketplace.ReasonReviewIncomplete {
		t.Fatalf("unreviewed specialist publish = %#v", blocked)
	}

	profile, err = store.SubmitEvidence(identityID, "anxiety", "DIPLOMA", "document:test-diploma")
	if err != nil {
		t.Fatal(err)
	}
	profile, err = store.SubmitEvidence(identityID, "anxiety", "DIPLOMA", "document:test-diploma")
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Evidence) != 1 {
		t.Fatalf("idempotent evidence intake produced duplicates: %#v", profile.Evidence)
	}
	if len(profile.Capabilities) != 1 ||
		profile.Capabilities[0].EvidenceState != marketplace.EvidenceDocumentSupported ||
		profile.Capabilities[0].VerificationState != "PENDING" {
		t.Fatalf("submitted evidence state = %#v", profile.Capabilities)
	}
	if profile.ReviewState != marketplace.ReviewManual {
		t.Fatalf("profile review state = %s want MANUAL_REVIEW", profile.ReviewState)
	}

	evidenceID := profile.Evidence[0].ID
	if err := store.ReviewEvidence(evidenceID, true, "reviewer:test"); err != nil {
		t.Fatal(err)
	}
	profile, err = store.Profile(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Capabilities[0].EvidenceState != marketplace.EvidenceAPGICVerified ||
		profile.Capabilities[0].VerificationState != "ACTIVE" {
		t.Fatalf("approved evidence did not verify capability: %#v", profile.Capabilities[0])
	}

	profile, err = store.SubmitEvidence(identityID, "anxiety", "CERTIFICATE", "document:supplemental")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Capabilities[0].EvidenceState != marketplace.EvidenceDocumentSupported ||
		profile.Capabilities[0].VerificationState != "PENDING" {
		t.Fatalf("supplemental evidence did not trigger re-review: %#v", profile.Capabilities[0])
	}
	var supplementalID string
	for _, item := range profile.Evidence {
		if item.Reference == "document:supplemental" {
			supplementalID = item.ID
			break
		}
	}
	if supplementalID == "" {
		t.Fatal("supplemental evidence not persisted")
	}
	if err := store.ReviewEvidence(supplementalID, false, "reviewer:test"); err != nil {
		t.Fatal(err)
	}
	profile, err = store.Profile(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Capabilities[0].EvidenceState != marketplace.EvidenceAPGICVerified ||
		profile.Capabilities[0].VerificationState != "ACTIVE" {
		t.Fatalf("rejected supplemental evidence erased prior accepted verification: %#v", profile.Capabilities[0])
	}

	blocked, err = store.Publish(identityID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Allowed || blocked.ReasonCodes[0] != marketplace.ReasonReviewIncomplete {
		t.Fatalf("capability review bypassed profile review gate: %#v", blocked)
	}

	if err := store.ReviewProfile(identityID, true, "reviewer:test"); err != nil {
		t.Fatal(err)
	}
	allowed, err := store.Publish(identityID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed || len(allowed.PublishedTopics) != 1 || allowed.PublishedTopics[0] != "anxiety" {
		t.Fatalf("approved specialist publish = %#v", allowed)
	}

	profile, err = store.Profile(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.PublishedTopics) != 1 || profile.PublishedTopics[0] != "anxiety" {
		t.Fatalf("durable publication missing: %#v", profile.PublishedTopics)
	}
	discoveryProfiles, err := store.PublishedProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var discoveryFound bool
	for _, candidate := range discoveryProfiles {
		if candidate.IdentityID == identityID && candidate.IsPublishedFor("anxiety") {
			discoveryFound = true
		}
	}
	if !discoveryFound {
		t.Fatalf("active publication missing from durable discovery snapshot: %#v", discoveryProfiles)
	}

	if _, err := store.Unpublish(identityID, "anxiety"); err != nil {
		t.Fatal(err)
	}
	profile, err = store.Profile(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.PublishedTopics) != 0 {
		t.Fatalf("unpublish left active discoverability: %#v", profile.PublishedTopics)
	}
	discoveryProfiles, err = store.PublishedProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range discoveryProfiles {
		if candidate.IdentityID == identityID {
			t.Fatalf("unpublished specialist remained discoverable: %#v", candidate)
		}
	}
}

func containsIdentityRole(roles []identity.Role, wanted identity.Role) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
