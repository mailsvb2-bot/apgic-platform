package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestBookingOwnerResumeHTTPRequiresSessionAndReadsCanonicalBooking(t *testing.T) {
	service := demand.NewConformanceService(nil)
	owner, err := service.CreateIntent("нужна помощь со сном")
	if err != nil { t.Fatal(err) }
	stranger, err := service.CreateIntent("другой клиент")
	if err != nil { t.Fatal(err) }
	if _, err := service.ConfirmIntent(owner.ID, []string{"sleep"}, nil, nil); err != nil { t.Fatal(err) }
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 { t.Fatalf("slots=%#v err=%v", slots, err) }
	hold, err := service.AcquireHold(owner.ID, slots[0].ID, owner.ClientIdentityID)
	if err != nil { t.Fatal(err) }
	sessionKey := []byte(strings.Repeat("b", 32))
	sessions, err := newClientSessionManager(sessionKey, time.Now)
	if err != nil { t.Fatal(err) }
	ownerCookie, err := sessions.issue(owner.ClientIdentityID)
	if err != nil { t.Fatal(err) }
	strangerCookie, err := sessions.issue(stranger.ClientIdentityID)
	if err != nil { t.Fatal(err) }
	handler := New(Options{Demand: service, ClientSessionKey: sessionKey})
	path := "/v1/bookings/" + hold.BookingID
	read := func(cookie *http.Cookie, url string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, url, nil)
		if cookie != nil { request.AddCookie(cookie) }
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, request)
		return rec
	}
	if response := read(nil, path); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated response=%d body=%s", response.Code, response.Body.String())
	}
	if response := read(strangerCookie, path); response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), owner.ClientIdentityID) {
		t.Fatalf("foreign owner leaked resource: status=%d body=%s", response.Code, response.Body.String())
	}
	first := read(ownerCookie, path)
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("canonical owner read status=%d cache=%q body=%s", first.Code, first.Header().Get("Cache-Control"), first.Body.String())
	}
	var held demand.BookingStatus
	if err := json.Unmarshal(first.Body.Bytes(), &held); err != nil { t.Fatal(err) }
	if held.BookingID != hold.BookingID || held.HoldID != hold.ID || held.State != booking.StateHeld {
		t.Fatalf("first status=%#v", held)
	}
	if strings.Contains(first.Body.String(), "payment_secret") || strings.Contains(first.Body.String(), "client_identity_id") {
		t.Fatalf("owner projection contains unnecessary sensitive fields: %s", first.Body.String())
	}
	if _, err := service.CreateCheckout(hold.ID, owner.ClientIdentityID, "SBP"); err != nil { t.Fatal(err) }
	second := read(ownerCookie, path)
	if second.Code != http.StatusOK {
		t.Fatalf("after checkout status=%d body=%s", second.Code, second.Body.String())
	}
	var pending demand.BookingStatus
	if err := json.Unmarshal(second.Body.Bytes(), &pending); err != nil { t.Fatal(err) }
	if pending.State != booking.StatePendingPayment || pending.StartsAt == nil || pending.EndsAt == nil {
		t.Fatalf("owner must see pending payment, not fake confirmation: %#v", pending)
	}
	if missing := read(ownerCookie, "/v1/bookings/does-not-exist"); missing.Code != http.StatusNotFound {
		t.Fatalf("missing booking status=%d", missing.Code)
	}
}
