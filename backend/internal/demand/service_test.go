package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
)

func TestMultipleHelpIntentsReuseOneCanonicalIdentity(t *testing.T) {
	service := NewConformanceService(nil)
	identityID := "11111111-1111-4111-8111-111111111111"

	first, err := service.CreateIntentForIdentity(identityID, "не могу уснуть")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntentForIdentity(identityID, "тревожно перед выступлением")
	if err != nil {
		t.Fatal(err)
	}

	if first.ID == second.ID {
		t.Fatal("separate help intents must keep distinct intent identity")
	}
	if first.ClientIdentityID != identityID || second.ClientIdentityID != identityID {
		t.Fatalf("help intents split account truth: first=%s second=%s", first.ClientIdentityID, second.ClientIdentityID)
	}
	if len(service.owners) != 1 {
		t.Fatalf("owner count = %d, want one canonical identity", len(service.owners))
	}
	owner := service.owners[identityID]
	if owner == nil || !owner.HasRole(identity.RoleClient) {
		t.Fatalf("canonical client identity missing: %#v", owner)
	}
}

func TestCreateIntentForIdentityRejectsMissingIdentity(t *testing.T) {
	service := NewConformanceService(nil)
	if _, err := service.CreateIntentForIdentity("", "нужна помощь"); !errors.Is(err, identity.ErrInvalidIdentity) {
		t.Fatalf("missing canonical identity err = %v", err)
	}
}

func TestInterpretationIsCorrectableAndNeverAssertsDiagnosis(t *testing.T) {
	fixed := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return fixed })

	intent, err := service.CreateIntent("Мне поставили диагноз и тревожно перед выступлениями")
	if err != nil {
		t.Fatal(err)
	}
	if intent.DiagnosisAsserted {
		t.Fatal("interpretation asserted a diagnosis")
	}
	if intent.Status != "DRAFT" || intent.Topics[0] != "anxiety" {
		t.Fatalf("draft = %#v", intent)
	}
	if intent.Notice == "" {
		t.Fatal("missing user-facing correction notice")
	}

	confirmed, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, []string{"restore-sleep"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "CONFIRMED" || confirmed.Topics[0] != "sleep" || confirmed.DiagnosisAsserted {
		t.Fatalf("confirmed = %#v", confirmed)
	}

	cards, topic, err := service.Matches(intent.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if topic != "sleep" {
		t.Fatalf("topic = %s", topic)
	}
	for _, card := range cards {
		if card.SpecialistID == "spec-sokolov" || card.SpecialistID == "spec-draft" {
			t.Fatalf("ineligible or anxiety-only specialist returned for sleep: %#v", cards)
		}
	}
	if len(cards) != 1 || cards[0].SpecialistID != "spec-lebedeva" {
		t.Fatalf("cards = %#v", cards)
	}
}

func TestSponsoredCandidateDoesNotOutrankHigherExpertise(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("Мне тревожно")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"anxiety"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	cards, _, err := service.Matches(intent.ID, "anxiety")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) < 2 || cards[0].SpecialistID != "spec-lebedeva" || cards[0].Sponsored {
		t.Fatalf("ranking = %#v", cards)
	}
	if cards[1].SpecialistID != "spec-sokolov" || !cards[1].Sponsored {
		t.Fatalf("sponsored peer missing: %#v", cards)
	}
}

func TestExclusiveHoldAllowsOnlyOneActiveClient(t *testing.T) {
	service := NewConformanceService(nil)
	first, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntent("тоже не сплю")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(first.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(second.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 || !slots[0].Exclusive {
		t.Fatalf("slots = %#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(first.ID, slots[0].ID, first.ClientIdentityID)
	if err != nil || hold.State != "ACTIVE" || hold.BookingState != "HELD" {
		t.Fatalf("hold = %#v err=%v", hold, err)
	}
	if service.bookings[hold.BookingID] != nil {
		t.Fatal("booking must not exist before checkout consumes the active hold")
	}
	again, err := service.AcquireHold(first.ID, slots[0].ID, first.ClientIdentityID)
	if err != nil || again.ID != hold.ID {
		t.Fatalf("idempotent hold = %#v err=%v", again, err)
	}
	if _, err := service.AcquireHold(second.ID, slots[0].ID, second.ClientIdentityID); !errors.Is(err, ErrSlotHeld) {
		t.Fatalf("second hold err = %v", err)
	}
}

func TestSlotsHideActiveHoldsFromAvailability(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	before, err := service.Slots("spec-lebedeva")
	if err != nil || len(before) < 2 {
		t.Fatalf("slots before hold=%#v err=%v", before, err)
	}
	hold, err := service.AcquireHold(intent.ID, before[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := service.Slots("spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range after {
		if slot.ID == hold.SlotID {
			t.Fatalf("active held slot %s remained advertised", hold.SlotID)
		}
	}
	if len(after) != len(before)-1 {
		t.Fatalf("available slots=%d want %d", len(after), len(before)-1)
	}
}

func TestUnconfirmedIntentCannotMatch(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("тревога")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Matches(intent.ID, "anxiety"); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("err = %v", err)
	}
}
