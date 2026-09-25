package demand

import (
	"errors"
	"testing"
	"time"
)

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
	again, err := service.AcquireHold(first.ID, slots[0].ID, first.ClientIdentityID)
	if err != nil || again.ID != hold.ID {
		t.Fatalf("idempotent hold = %#v err=%v", again, err)
	}
	if _, err := service.AcquireHold(second.ID, slots[0].ID, second.ClientIdentityID); !errors.Is(err, ErrSlotHeld) {
		t.Fatalf("second hold err = %v", err)
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
