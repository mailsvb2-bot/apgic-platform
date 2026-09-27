package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestJourneyStoreSurvivesServiceRestart(t *testing.T) {
	databaseURL := os.Getenv("APGIC_JOURNEY_STORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("journey integration database not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store := &Checker{db: db}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Second)
	var intentIDs []string
	var identityIDs []string
	var holdIDs []string
	var bookingIDs []string
	defer func() {
		for _, bookingID := range bookingIDs {
			_, _ = store.db.Exec("DELETE FROM bookings WHERE id = $1::uuid", bookingID)
		}
		for _, holdID := range holdIDs {
			_, _ = store.db.Exec("DELETE FROM booking_holds WHERE id = $1::uuid", holdID)
		}
		for _, intentID := range intentIDs {
			_, _ = store.db.Exec("DELETE FROM help_intents WHERE id = $1::uuid", intentID)
		}
		for _, identityID := range identityIDs {
			_, _ = store.db.Exec("DELETE FROM identity_roles WHERE identity_id = $1::uuid", identityID)
			_, _ = store.db.Exec("DELETE FROM identities WHERE id = $1::uuid", identityID)
		}
	}()

	first, err := demand.NewConformanceServiceWithStores(func() time.Time { return now }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := first.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	intentIDs = append(intentIDs, intent.ID)
	identityIDs = append(identityIDs, intent.ClientIdentityID)
	if _, err := first.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := first.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := first.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	holdIDs = append(holdIDs, hold.ID)
	bookingIDs = append(bookingIDs, hold.BookingID)

	secondNow := now.Add(time.Minute)
	second, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	matches, _, err := second.Matches(intent.ID, "sleep")
	if err != nil || len(matches) == 0 {
		t.Fatalf("hydrated intent matches=%#v err=%v", matches, err)
	}
	replayedHold, err := second.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatalf("hydrated active hold replay err=%v", err)
	}
	if replayedHold.ID != hold.ID || replayedHold.BookingID != hold.BookingID {
		t.Fatalf("hydrated hold changed identity: got=%#v want=%#v", replayedHold, hold)
	}
	instruction, err := second.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	if instruction.BookingID != hold.BookingID || instruction.BookingState != "PENDING_PAYMENT" {
		t.Fatalf("checkout=%#v", instruction)
	}

	third, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow.Add(time.Minute) }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	other, err := third.CreateIntent("тоже нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	intentIDs = append(intentIDs, other.ID)
	identityIDs = append(identityIDs, other.ClientIdentityID)
	if _, err := third.ConfirmIntent(other.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := third.AcquireHold(other.ID, slots[0].ID, other.ClientIdentityID); !errors.Is(err, demand.ErrSlotBooked) {
		t.Fatalf("restart lost live booking exclusivity: err=%v", err)
	}
}
