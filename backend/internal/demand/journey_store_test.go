package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

type fakeJourneyStore struct {
	createIntentErr   error
	confirmIntentErr  error
	acquireHoldErr    error
	acquireHoldReason string
	createBookingErr  error
	createBookReason  string
	expireErr         error
	updateBookingErr  error
}

func (f *fakeJourneyStore) BootstrapCatalog(slots []Slot) ([]Slot, error) {
	return append([]Slot(nil), slots...), nil
}

func (f *fakeJourneyStore) LoadJourney([]Slot) (JourneySnapshot, error) {
	return JourneySnapshot{}, nil
}

func (f *fakeJourneyStore) CreateIntent(*Intent) error {
	return f.createIntentErr
}

func (f *fakeJourneyStore) ConfirmIntent(*Intent, time.Time) error {
	return f.confirmIntentErr
}

func (f *fakeJourneyStore) AcquireHold(*Hold, Slot, time.Time) (string, error) {
	if f.acquireHoldReason != "" {
		return f.acquireHoldReason, f.acquireHoldErr
	}
	return "BOOK_HOLD_ACQUIRED", f.acquireHoldErr
}

func (f *fakeJourneyStore) CreateBooking(*booking.Booking, time.Time) (string, error) {
	if f.createBookReason != "" {
		return f.createBookReason, f.createBookingErr
	}
	return "BOOK_CREATED", f.createBookingErr
}

func (f *fakeJourneyStore) Expire(time.Time) error {
	return f.expireErr
}

func (f *fakeJourneyStore) UpdateBooking(*booking.Booking) error {
	return f.updateBookingErr
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

func TestCreateBookingStoreFailureLeavesHoldActive(t *testing.T) {
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
	store.createBookingErr = errors.New("db down")
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); err == nil {
		t.Fatal("checkout must fail when durable booking create fails")
	}
	stored := service.holds[hold.ID]
	if stored == nil || stored.State != "ACTIVE" || stored.BookingState != booking.StateHeld {
		t.Fatalf("failed booking create mutated hold=%#v", stored)
	}
	if len(service.bookings) != 0 || len(service.instructions) != 0 {
		t.Fatalf("failed booking create committed memory: bookings=%d instructions=%d", len(service.bookings), len(service.instructions))
	}
}
