package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

type fakeJourneyStore struct {
	snapshot             JourneySnapshot
	createIntentErr      error
	confirmIntentErr     error
	acquireHoldErr       error
	acquireHoldReason    string
	createCheckoutErr    error
	createCheckoutReason string
	expireErr            error
}

func (f *fakeJourneyStore) BootstrapCatalog(slots []Slot) ([]Slot, error) {
	return append([]Slot(nil), slots...), nil
}

func (f *fakeJourneyStore) LoadJourney([]Slot) (JourneySnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeJourneyStore) CreateIntent(intent *Intent) error {
	if f.createIntentErr != nil {
		return f.createIntentErr
	}
	f.snapshot.Intents = append(f.snapshot.Intents, cloneIntent(intent))
	return nil
}

func (f *fakeJourneyStore) ConfirmIntent(intent *Intent, _ time.Time) error {
	if f.confirmIntentErr != nil {
		return f.confirmIntentErr
	}
	for index, existing := range f.snapshot.Intents {
		if existing != nil && existing.ID == intent.ID {
			f.snapshot.Intents[index] = cloneIntent(intent)
			return nil
		}
	}
	f.snapshot.Intents = append(f.snapshot.Intents, cloneIntent(intent))
	return nil
}

func (f *fakeJourneyStore) AcquireHold(hold *Hold, _ Slot, _ time.Time) (string, error) {
	if f.acquireHoldErr != nil {
		return "", f.acquireHoldErr
	}
	if f.acquireHoldReason != "" {
		return f.acquireHoldReason, nil
	}
	copyHold := *hold
	f.snapshot.Holds = append(f.snapshot.Holds, &copyHold)
	return "BOOK_HOLD_ACQUIRED", nil
}

func (f *fakeJourneyStore) CreateCheckout(persistence CheckoutPersistence) (string, error) {
	if f.createCheckoutErr != nil {
		return "", f.createCheckoutErr
	}
	if f.createCheckoutReason != "" {
		return f.createCheckoutReason, nil
	}
	if persistence.Booking != nil {
		copyBooking := *persistence.Booking
		f.snapshot.Bookings = append(f.snapshot.Bookings, &copyBooking)
	}
	if persistence.Instruction != nil {
		copyInstruction := *persistence.Instruction
		f.snapshot.Instructions = append(f.snapshot.Instructions, &copyInstruction)
		for _, hold := range f.snapshot.Holds {
			if hold != nil && hold.ID == persistence.Instruction.HoldID {
				hold.State = "CONSUMED"
				hold.BookingState = persistence.Instruction.BookingState
			}
		}
	}
	return "BOOK_CREATED", nil
}

func (f *fakeJourneyStore) Expire(time.Time) error {
	return f.expireErr
}

func TestJourneyReasonErrorRejectsUnknownProtocolReason(t *testing.T) {
	err := journeyReasonError("BOOK_NEW_UNKNOWN_REASON")
	if !errors.Is(err, ErrJourneyStoreProtocol) {
		t.Fatalf("unknown reason err=%v", err)
	}
}

func TestCreateIntentStoreFailureDoesNotMutateMemory(t *testing.T) {
	store := &fakeJourneyStore{createIntentErr: errors.New("db down")}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateIntent("бессонница"); err == nil {
		t.Fatal("create intent must fail when durable store fails")
	}
	if len(service.intents) != 0 || len(service.owners) != 0 {
		t.Fatalf("failed durable create mutated memory: intents=%d owners=%d", len(service.intents), len(service.owners))
	}
}

func TestConfirmIntentStoreFailureKeepsDraft(t *testing.T) {
	store := &fakeJourneyStore{}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	store.confirmIntentErr = errors.New("db down")
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err == nil {
		t.Fatal("confirm must fail when durable store fails")
	}
	stored := service.intents[intent.ID]
	if stored == nil || stored.Status != "DRAFT" {
		t.Fatalf("failed durable confirm mutated intent=%#v", stored)
	}
}

func TestAcquireHoldStoreFailureDoesNotReserveSlotInMemory(t *testing.T) {
	store := &fakeJourneyStore{}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := service.CreateIntent("бессонница")
	_, _ = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil)
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	store.acquireHoldErr = errors.New("db down")
	if _, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID); err == nil {
		t.Fatal("hold must fail when durable store fails")
	}
	if len(service.holds) != 0 || len(service.slotHolds) != 0 {
		t.Fatalf("failed durable hold mutated memory: holds=%d slotHolds=%d", len(service.holds), len(service.slotHolds))
	}
}

func TestCreateCheckoutStoreFailureLeavesHoldActive(t *testing.T) {
	store := &fakeJourneyStore{}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := service.CreateIntent("бессонница")
	_, _ = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil)
	slots, _ := service.Slots("spec-lebedeva")
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	store.createCheckoutErr = errors.New("db down")
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); err == nil {
		t.Fatal("checkout must fail when durable checkout create fails")
	}
	stored := service.holds[hold.ID]
	if stored == nil || stored.State != "ACTIVE" || stored.BookingState != booking.StateHeld {
		t.Fatalf("failed booking create mutated hold=%#v", stored)
	}
	if len(service.bookings) != 0 || len(service.instructions) != 0 {
		t.Fatalf("failed booking create committed memory: bookings=%d instructions=%d", len(service.bookings), len(service.instructions))
	}
}

type liveJourneyStoreProbe struct {
	fakeJourneyStore
	bootstrapCalls int
}

func (f *liveJourneyStoreProbe) LoadLiveJourney() (JourneySnapshot, error) {
	return f.snapshot, nil
}

func (f *liveJourneyStoreProbe) BootstrapCatalog(slots []Slot) ([]Slot, error) {
	f.bootstrapCalls++
	return f.fakeJourneyStore.BootstrapCatalog(slots)
}

func TestLiveSupplyNeverBootstrapsOrBooksConformanceSpecialists(t *testing.T) {
	store := &liveJourneyStoreProbe{}
	profile, err := marketplace.NewSpecialistProfile(
		"approved-live-specialist", "live-specialist-identity", "Проверенная Анна",
	)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := marketplace.NewCapability(
		"anxiety", marketplace.EvidenceAPGICVerified, "reviewed:live",
	)
	if err != nil {
		t.Fatal(err)
	}
	profile.AddCapability(capability)
	profile.Complete = true
	profile.Profession = "PSYCHOLOGIST"
	profile.Review = marketplace.ReviewApproved
	profile.PublishState = marketplace.PublishPublished
	profile.PublishedTopics = []string{"anxiety"}
	supply := publishedSpecialistStoreStub{profiles: []marketplace.SpecialistProfile{*profile}}

	service, err := NewLiveServiceWithStoresAndSpecialists(nil, nil, store, supply)
	if err != nil {
		t.Fatal(err)
	}
	if store.bootstrapCalls != 0 {
		t.Fatalf("production seeded %d conformance catalogs", store.bootstrapCalls)
	}
	if service.CatalogMode() != CatalogModeLive || len(service.catalog.candidates) != 0 ||
		len(service.catalog.slots) != 0 || len(service.catalog.names) != 0 {
		t.Fatalf("production inherited synthetic provider truth: %#v", service.catalog)
	}
	view, err := service.Search("anxiety")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, entry := range view.Entries {
		if entry.SpecialistID == "spec-sokolov" || entry.SpecialistID == "spec-lebedeva" {
			t.Fatalf("fixture specialist leaked into live public search: %#v", view.Entries)
		}
		if entry.SpecialistID == profile.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("approved durable profile absent from live discovery: %#v", view.Entries)
	}
	intent, err := service.CreateIntent("нужна помощь при тревоге")
	if err != nil {
		t.Fatal(err)
	}
	if intent.CatalogMode != CatalogModeLive {
		t.Fatalf("live intent mode = %s", intent.CatalogMode)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"anxiety"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	cards, topic, err := service.Matches(intent.ID, "anxiety")
	if err != nil || topic != "anxiety" || len(cards) != 0 {
		t.Fatalf("live supply invented bookable offer before real price/slot: %#v topic=%s err=%v", cards, topic, err)
	}
	for _, demoID := range []string{"spec-lebedeva", "spec-sokolov", "spec-kim", "spec-volkova"} {
		if _, err := service.Slots(demoID); !errors.Is(err, ErrSpecialistNotFound) {
			t.Fatalf("live catalog exposed fabricated slots for %s: err=%v", demoID, err)
		}
	}
	if _, err := service.Slots(profile.ID); !errors.Is(err, ErrSpecialistNotFound) {
		t.Fatalf("published profile without certified offer became bookable: %v", err)
	}
	if store.bootstrapCalls != 0 {
		t.Fatal("live user journey bootstrapped demo slots")
	}
	restarted, err := NewLiveServiceWithStoresAndSpecialists(nil, nil, store, supply)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := restarted.intents[intent.ID]
	if reloaded == nil || reloaded.CatalogMode != CatalogModeLive {
		t.Fatalf("live intent not restored after restart: %#v", reloaded)
	}
}

func TestLiveSupplyRejectsMissingDurableOwners(t *testing.T) {
	if _, err := NewLiveServiceWithStoresAndSpecialists(nil, nil, nil, nil); err == nil {
		t.Fatal("live startup accepted absent durable owners")
	}
}
