package runtimepostgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// Real PostgreSQL owners only; no conformance profile, Product, offer or price.
// This test must run against migrated PostgreSQL and never a fake checkout.
func TestLivePricedSpecialistToDurableHoldAfterRestart(t *testing.T) {
	databaseURL := os.Getenv("APGIC_SPECIALIST_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("specialist PostgreSQL integration URL is not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	specialistIdentityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.UpsertProfile(specialistIdentityID, "Тестовый проверенный специалист", "PSYCHOLOGIST")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeclareCapability(specialistIdentityID, "anxiety"); err != nil {
		t.Fatal(err)
	}
	profile, err := store.SubmitEvidence(specialistIdentityID, "anxiety", "DIPLOMA", "test-reviewed-document:live")
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Evidence) == 0 {
		t.Fatal("missing real evidence row")
	}
	if err := store.ReviewEvidence(profile.Evidence[0].ID, true, "reviewer:postgres-live-test"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewProfile(specialistIdentityID, true, "reviewer:postgres-live-test"); err != nil {
		t.Fatal(err)
	}
	published, err := store.Publish(specialistIdentityID, "anxiety")
	if err != nil || !published.Allowed {
		t.Fatalf("publish reviewed specialist: %#v, %v", published, err)
	}
	organization, err := store.CreateOrganization(specialistIdentityID, "Live-priced specialist organization")
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.CreateOrganizationDirection(specialistIdentityID, organization.ID, "Consultations", "CONSULTATION")
	if err != nil || len(organization.Directions) == 0 {
		t.Fatalf("create actual consultation direction: %#v, %v", organization, err)
	}
	product, err := store.CreateOrganizationProduct(specialistIdentityID, organization.ID, commerce.OrganizationProductDraft{
		Name:                  "Individual consultation",
		DirectionID:           organization.Directions[0].ID,
		CommercialOwnerRef:    "organization/" + organization.ID,
		AuthorRefs:            []string{"identity/" + specialistIdentityID},
		RevenueBeneficiaryRef: "identity/" + specialistIdentityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBookableOffer(specialistIdentityID, product.ID, "anxiety", 190000); !errors.Is(err, specialist.ErrOfferNotAuthorized) {
		// Draft Product is not authorized for payable offer.
		t.Fatalf("draft product accepted for live offer: %v", err)
	}
	if _, err := store.PublishOrganizationProduct(specialistIdentityID, organization.ID, product.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	offer, err := store.CreateBookableOffer(specialistIdentityID, product.ID, "anxiety", 190000)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	end := start.Add(50 * time.Minute)
	slot, err := store.AddBookableSlot(specialistIdentityID, offer.ID, start, end)
	if err != nil || slot.OfferID != offer.ID {
		t.Fatalf("persist real priced slot: %#v, err=%v", slot, err)
	}
	if _, err := store.AddBookableSlot(specialistIdentityID, offer.ID, start.Add(10*time.Minute), end.Add(10*time.Minute)); !errors.Is(err, specialist.ErrSlotConflict) {
		t.Fatalf("overlapping live slot not blocked: %v", err)
	}
	if _, err := store.AddBookableSlot("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", offer.ID, start.Add(time.Hour), end.Add(time.Hour)); !errors.Is(err, specialist.ErrOfferNotAuthorized) {
		t.Fatalf("foreign identity mutated offer slot: %v", err)
	}
	publicBeforeHold, err := store.PublicOffers(context.Background(), created.ID)
	if err != nil || len(publicBeforeHold) != 1 || publicBeforeHold[0].ID != offer.ID ||
		publicBeforeHold[0].PriceMinor != 190000 || publicBeforeHold[0].Currency != "RUB" ||
		len(publicBeforeHold[0].Slots) != 1 || publicBeforeHold[0].Slots[0].SlotID != slot.ID {
		t.Fatalf("public directory fabricated or omitted live priced slot: %#v err=%v", publicBeforeHold, err)
	}
	offers, err := store.BookableOffers()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range offers {
		if candidate.ID == offer.ID {
			if candidate.PriceMinor != 190000 || candidate.Profile.ID != created.ID ||
				len(candidate.Slots) != 1 || candidate.Slots[0].ID != slot.ID {
				t.Fatalf("live offer not sourced from canonical price and slot: %#v", candidate)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("real priced specialist absent from PostgreSQL supply")
	}

	service, err := demand.NewLiveServiceWithStoresAndSpecialists(nil, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	clientIdentityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	intent, err := service.CreateIntentForIdentity(clientIdentityID, "мне тревожно")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"anxiety"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	matches, topic, err := service.Matches(intent.ID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	bookable := false
	for _, match := range matches {
		if match.OfferID == offer.ID {
			if topic != "anxiety" || match.PriceMinor != offer.PriceMinor || match.Sponsored {
				t.Fatalf("paid specialist fabricated or mispriced: %#v", match)
			}
			bookable = true
		}
	}
	if !bookable {
		t.Fatal("verified priced specialist not actionable in live match result")
	}
	slots, err := service.SlotsForOffer(created.ID, offer.ID)
	if err != nil || len(slots) != 1 || slots[0].ID != slot.ID {
		t.Fatalf("live slot list = %#v, err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slot.ID, clientIdentityID)
	if err != nil {
		t.Fatalf("real live slot hold failed: %v", err)
	}
	// Public price and availability are computed against the same canonical held
	// slot state as the client journey. Once held, a slot cannot remain advertised.
	publicAfterHold, err := store.PublicOffers(context.Background(), created.ID)
	if err != nil || len(publicAfterHold) != 0 {
		t.Fatalf("held slot remained in public availability: %#v err=%v", publicAfterHold, err)
	}
	// Simulate a new API process with the same PostgreSQL: readback cannot
	// depend on a browser tab-local cache or the old in-memory service.
	restarted, err := demand.NewLiveServiceWithStoresAndSpecialists(nil, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	status, err := restarted.ReadBookingStatus(hold.BookingID, clientIdentityID)
	if err != nil || status == nil || status.State != "HELD" ||
		status.HoldID != hold.ID {
		t.Fatalf("live booking vanished after API restart: %#v, err=%v", status, err)
	}
	if _, err := restarted.ReadBookingStatus(hold.BookingID, specialistIdentityID); !errors.Is(err, demand.ErrBookingNotFound) {
		t.Fatalf("cross-owner booking status leaked: %v", err)
	}
	paused, err := store.PauseBookableOffer(specialistIdentityID, offer.ID)
	if err != nil || paused.State != "PAUSED" {
		t.Fatalf("live offer pause = %#v, err=%v", paused, err)
	}
	paused, err = store.PauseBookableOffer(specialistIdentityID, offer.ID)
	if err != nil || paused.State != "PAUSED" {
		t.Fatalf("idempotent pause failed: %#v, err=%v", paused, err)
	}
	afterPause, err := store.BookableOffers()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range afterPause {
		if item.ID == offer.ID {
			t.Fatal("paused offer still purchasable")
		}
	}
	// Pause does not erase historical durable holds already issued to a client.
	afterRestart, err := demand.NewLiveServiceWithStoresAndSpecialists(nil, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	status, err = afterRestart.ReadBookingStatus(hold.BookingID, clientIdentityID)
	if err != nil || status == nil || status.HoldID != hold.ID {
		t.Fatalf("paused offer erased historical hold: %#v, err=%v", status, err)
	}
	updatedOffer, err := store.CreateBookableOffer(specialistIdentityID, product.ID, "anxiety", 240000)
	if err != nil || updatedOffer.PriceMinor != 240000 || updatedOffer.ID == offer.ID {
		t.Fatalf("immutable price could not be corrected via new version: %#v, err=%v", updatedOffer, err)
	}
}
