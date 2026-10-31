package runtimepostgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// This is an actual PostgreSQL owner/readback test, never a mocked Web route.
// It proves that the personal workspace does not invent a completed booking,
// survives a fresh API store connection and cannot read another identity's data.
func TestClientPersonalOwnerIsolationAndRestartReadback(t *testing.T) {
	databaseURL := os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("client personal PostgreSQL integration URL is not configured")
	}
	ctx := context.Background()
	first, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{owner, foreign} {
		if _, err := first.db.ExecContext(ctx, "INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	prefs := clientpersonal.Preferences{DisplayName: "Проверенный клиент", Locale: "ru", TimeZone: "Europe/Moscow"}
	if _, err := first.UpdateClientPersonalPreferences(ctx, owner, prefs); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a, err := first.ReadClientPersonal(ctx, owner, 10, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.IdentityID != owner || a.Preferences.DisplayName != prefs.DisplayName ||
		len(a.Upcoming) != 0 || len(a.History) != 0 {
		t.Fatalf("personal read invented a booking or lost preferences: %#v", a)
	}
	b, err := first.ReadClientPersonal(ctx, foreign, 10, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if b.IdentityID != foreign || b.Preferences.DisplayName != "" ||
		len(b.Upcoming) != 0 || len(b.History) != 0 || len(b.Requests) != 0 {
		t.Fatalf("foreign identity accessed another owner's facts: %#v", b)
	}
	unknown, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ReadClientPersonal(ctx, unknown, 10, nil, now); !errors.Is(err, clientpersonal.ErrIdentityNotFound) {
		t.Fatalf("unknown signed identity read personal data: %v", err)
	}
	missingBooking, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	checkin := clientpersonal.CheckInInput{BookingID: missingBooking, ProgressCode: "UNSURE"}
	if _, err := first.CreateClientCheckIn(ctx, owner, checkin); !errors.Is(err, clientpersonal.ErrCheckInNotEligible) {
		t.Fatalf("client minted a provider-completed report without booking: %v", err)
	}
	if _, err := first.CreateClientCheckIn(ctx, owner, clientpersonal.CheckInInput{BookingID: missingBooking, ProgressCode: "CURED"}); !errors.Is(err, clientpersonal.ErrCheckInInvalid) {
		t.Fatalf("clinical claim accepted as client-reported nonmedical outcome: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	after, err := restarted.ReadClientPersonal(ctx, owner, 10, nil, now.Add(time.Minute))
	if err != nil || after.IdentityID != owner || after.Preferences.DisplayName != prefs.DisplayName ||
		len(after.History) != 0 {
		t.Fatalf("verified owner readback failed across API store restart: %#v, %v", after, err)
	}
	again, err := restarted.ReadClientPersonal(ctx, foreign, 10, nil, now.Add(time.Minute))
	if err != nil || again.Preferences.DisplayName != "" {
		t.Fatalf("restart disclosed owner data across identities: %#v, %v", again, err)
	}
}
