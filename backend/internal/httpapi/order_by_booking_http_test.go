package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestOrderRecoveredFromBookingRequiresSignedSession(t *testing.T) {
	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("r", 32))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := sessions.issue(intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := sessions.issue("foreign-client")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{Demand: service, ClientSessionKey: key, ConformanceProviderEvents: true})
	path := "/v1/bookings/" + hold.BookingID + "/order"
	call := func(cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct {
		cookie *http.Cookie
		code   int
	}{
		{nil, 401}, {foreign, 404}, {owner, 404},
	} {
		rec := call(tc.cookie)
		if rec.Code != tc.code {
			t.Fatalf("unpaid, absent or foreign status=%d want=%d body=%s", rec.Code, tc.code, rec.Body)
		}
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	rec := call(owner)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), instruction.OrderID) {
		t.Fatalf("owner order recovery=%d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("financial status must not be stored by intermediary caches")
	}
	if rec := call(foreign); rec.Code != 404 {
		t.Fatalf("foreign booking disclosed payment: HTTP %d", rec.Code)
	}
}
