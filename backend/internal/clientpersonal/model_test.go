package clientpersonal

import (
	"errors"
	"testing"
	"time"
)

func TestHistoryCursorIsStrictAndRoundTrips(t *testing.T) {
	at := time.Date(2026, 10, 12, 11, 22, 33, 900, time.UTC)
	id := "123e4567-e89b-42d3-a456-426614174000"
	raw := EncodeCursor(at, id)
	got, err := DecodeCursor(raw)
	if err != nil || got.BookingID != id || !got.StartsAt.Equal(at) {
		t.Fatalf("cursor roundtrip: %+v %v", got, err)
	}
	for _, bad := range []string{"garbage", "aGVsbG8", "", "bad|x"} {
		if bad == "" {
			if v, e := DecodeCursor(bad); e != nil || v != nil {
				t.Fatal("empty cursor")
			}
			continue
		}
		if _, e := DecodeCursor(bad); !errors.Is(e, ErrInvalidCursor) {
			t.Fatalf("accepted cursor %q: %v", bad, e)
		}
	}
}
func TestPersonalPreferencesRejectUnknownTimezoneAndControls(t *testing.T) {
	p := DefaultPreferences()
	if err := ValidatePreferences(p); err != nil {
		t.Fatal(err)
	}
	p.TimeZone = "Invalid/Zone"
	if !errors.Is(ValidatePreferences(p), ErrInvalidPreferences) {
		t.Fatal("unknown time zone accepted")
	}
	p = DefaultPreferences()
	p.DisplayName = "Alice\nSecret"
	if !errors.Is(ValidatePreferences(p), ErrInvalidPreferences) {
		t.Fatal("newline accepted")
	}
}
