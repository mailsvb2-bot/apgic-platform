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
	if len(firstIDs) != 8 || len(againIDs) != 8 {
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
	if len(nextIDs) != 8 {
		t.Fatalf("next-day slot count=%d", len(nextIDs))
	}
	for i := 0; i < 7; i++ {
		if firstIDs[i+1] != nextIDs[i] {
			t.Fatalf("rolling overlap mismatch at %d: first=%v next=%v", i, firstIDs, nextIDs)
		}
	}
	if firstIDs[7] == nextIDs[7] {
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
	if err != nil || len(first) != 8 {
		t.Fatalf("initial slots=%#v err=%v", first, err)
	}

	now = now.AddDate(0, 0, 9)
	next, err := service.Slots("spec-lebedeva")
	if err != nil || len(next) != 8 {
		t.Fatalf("refreshed slots=%#v err=%v", next, err)
	}
	if !next[0].StartsAt.After(now) {
		t.Fatalf("refreshed horizon starts in the past: %#v", next[0])
	}
	if first[0].ID == next[0].ID {
		t.Fatalf("long-lived service did not roll slot horizon: first=%s next=%s", first[0].ID, next[0].ID)
	}
}
