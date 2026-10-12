package demand

import (
	"testing"
	"time"
)

func TestConformanceCatalogUsesStableRollingFutureSlotRefs(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	first := conformanceCatalog(now)
	again := conformanceCatalog(now.Add(30 * time.Minute))

	var firstIDs []string
	for _, slot := range first.slots {
		if slot.SpecialistID != "spec-lebedeva" {
			continue
		}
		if !slot.StartsAt.After(now) {
			t.Fatalf("slot is not future: %#v", slot)
		}
		firstIDs = append(firstIDs, slot.ID)
	}
	var againIDs []string
	for _, slot := range again.slots {
		if slot.SpecialistID == "spec-lebedeva" {
			againIDs = append(againIDs, slot.ID)
		}
	}
	if len(firstIDs) != conformanceDays * conformanceSlotsPerDay || len(againIDs) != conformanceDays * conformanceSlotsPerDay {
		t.Fatalf("slot counts first=%d again=%d", len(firstIDs), len(againIDs))
	}
	for i := range firstIDs {
		if firstIDs[i] != againIDs[i] {
			t.Fatalf("same-day slot ref changed: first=%v again=%v", firstIDs, againIDs)
		}
	}

	nextDay := conformanceCatalog(now.Add(24 * time.Hour))
	var nextIDs []string
	for _, slot := range nextDay.slots {
		if slot.SpecialistID == "spec-lebedeva" {
			nextIDs = append(nextIDs, slot.ID)
		}
	}
	if len(nextIDs) != conformanceDays * conformanceSlotsPerDay {
		t.Fatalf("next-day slot count=%d", len(nextIDs))
	}
	for i := 0; i < (conformanceDays-1)*conformanceSlotsPerDay; i++ {
		if firstIDs[i+conformanceSlotsPerDay] != nextIDs[i] {
			t.Fatalf("rolling overlap mismatch at %d: first=%v next=%v", i, firstIDs, nextIDs)
		}
	}
	if firstIDs[len(firstIDs)-1] == nextIDs[len(nextIDs)-1] {
		t.Fatalf("rolling horizon did not replenish: first=%v next=%v", firstIDs, nextIDs)
	}
}

func TestConformanceCatalogSkipsSlotInsideHoldTTL(t *testing.T) {
	tooLate := time.Date(2026, 9, 27, 9, 50, 0, 0, time.UTC)
	catalog := conformanceCatalog(tooLate)
	for _, slot := range catalog.slots {
		if slot.SpecialistID != "spec-lebedeva" {
			continue
		}
		want := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
		if !slot.StartsAt.Equal(want) {
			t.Fatalf("first slot inside hold TTL was advertised: got=%s want=%s", slot.StartsAt, want)
		}
		break
	}

	earlyEnough := time.Date(2026, 9, 27, 9, 40, 0, 0, time.UTC)
	catalog = conformanceCatalog(earlyEnough)
	for _, slot := range catalog.slots {
		if slot.SpecialistID != "spec-lebedeva" {
			continue
		}
		want := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
		if !slot.StartsAt.Equal(want) {
			t.Fatalf("valid same-day slot was skipped: got=%s want=%s", slot.StartsAt, want)
		}
		break
	}
}

func TestLongLivedServiceRefreshesRollingSlotHorizon(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
	first, err := service.Slots("spec-lebedeva")
	if err != nil || len(first) != conformanceDays * conformanceSlotsPerDay {
		t.Fatalf("initial slots=%#v err=%v", first, err)
	}

	now = now.AddDate(0, 0, 9)
	next, err := service.Slots("spec-lebedeva")
	if err != nil || len(next) != conformanceDays * conformanceSlotsPerDay {
		t.Fatalf("refreshed slots=%#v err=%v", next, err)
	}
	if !next[0].StartsAt.After(now) {
		t.Fatalf("refreshed horizon starts in the past: %#v", next[0])
	}
	if first[0].ID == next[0].ID {
		t.Fatalf("long-lived service did not roll slot horizon: first=%s next=%s", first[0].ID, next[0].ID)
	}
}

func TestSharedConformanceBookingCatalogSustainsIndependentJourneys(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
	used := map[string]bool{}
	// All web viewport projects share a single conformance server. Exhausting
	// eight fixture slots must not masquerade as a broken real booking journey.
	for client := 0; client < 21; client++ {
		intent, err := service.CreateIntent("бессонница")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
			t.Fatal(err)
		}
		slots, err := service.Slots("spec-lebedeva")
		if err != nil || len(slots) < conformanceDays*conformanceSlotsPerDay-client {
			t.Fatalf("client %d, available slots=%d err=%v", client, len(slots), err)
		}
		if used[slots[0].ID] {
			t.Fatalf("slot %s re-advertised after exclusive hold", slots[0].ID)
		}
		hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
		if err != nil {
			t.Fatalf("client %d cannot acquire exclusive hold: %v", client, err)
		}
		used[slots[0].ID] = true
		if hold.SlotID != slots[0].ID || hold.ClientIdentityID != intent.ClientIdentityID {
			t.Fatalf("hold ownership broken: %#v", hold)
		}
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) < 21 {
		t.Fatalf("conformance suite ran out of bookable slots: len=%d err=%v", len(slots), err)
	}
	// These extra fixtures have no effect on production: liveCatalog itself
	// contains no invented specialists, offers, prices or appointment slots.
	live := liveCatalog()
	if len(live.slots) != 0 || len(live.candidates) != 0 {
		t.Fatal("conformance appointments leaked into real specialist supply")
	}
}
