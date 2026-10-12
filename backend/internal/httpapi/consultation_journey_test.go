package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

const journeyBookingID = "00000000-0000-4000-8000-000000000001"
const journeyIdentityID = "00000000-0000-4000-8000-000000000002"

type journeyStub struct {
	readyWrites, outcomeWrites int
	completed                  bool
}

func (s *journeyStub) ReadConsultationReady(_ context.Context, b, i string) (*connector.ReadySnapshot, error) {
	if i != journeyIdentityID || b != journeyBookingID {
		return nil, nil
	}
	return &connector.ReadySnapshot{BookingID: b, Role: "CLIENT", BookingState: "CONFIRMED"}, nil
}
func (s *journeyStub) ConfirmConsultationReady(ctx context.Context, b, i string, _ time.Time) (*connector.ReadySnapshot, error) {
	s.readyWrites++
	return s.ReadConsultationReady(ctx, b, i)
}
func (s *journeyStub) ReadConsultationOutcome(_ context.Context, b, i string) (*connector.OutcomeReport, error) {
	if i != journeyIdentityID {
		return nil, nil
	}
	return &connector.OutcomeReport{BookingID: b, Progress: 1, MatchSatisfaction: 5}, nil
}
func (s *journeyStub) RecordConsultationOutcome(ctx context.Context, b, i, k string, p, m int, _ time.Time) (*connector.OutcomeReport, error) {
	if !s.completed {
		return nil, connector.ErrConsultationOutcomeNotReady
	}
	s.outcomeWrites++
	return s.ReadConsultationOutcome(ctx, b, i)
}
func TestConsultationJourneySignedSessionAndEvidence(t *testing.T) {
	stub := &journeyStub{}
	key := []byte("consultation-tests-use-an-isolated-long-session-key")
	sessions, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(journeyIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Options{ClientSessionKey: key, ConsultationJourney: stub})
	call := func(method, path, body string, signed, mutation bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if signed {
			req.AddCookie(cookie)
		}
		if mutation {
			req.Header.Set("X-APGIC-Request", "consultation-v1")
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	base := "/v1/consultations/" + journeyBookingID
	if rr := call("POST", base+"/ready", "", false, true); rr.Code != http.StatusUnauthorized || stub.readyWrites != 0 {
		t.Fatalf("anonymous readiness status=%d writes=%d", rr.Code, stub.readyWrites)
	}
	if rr := call("POST", base+"/ready", "", true, false); rr.Code != http.StatusForbidden || stub.readyWrites != 0 {
		t.Fatalf("cross-site readiness status=%d writes=%d", rr.Code, stub.readyWrites)
	}
	if rr := call("POST", base+"/ready", "", true, true); rr.Code != http.StatusOK || stub.readyWrites != 1 {
		t.Fatalf("trusted readiness status=%d writes=%d body=%s", rr.Code, stub.readyWrites, rr.Body.String())
	}
	if rr := call("GET", base+"/ready", "", true, false); rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("readiness read status=%d", rr.Code)
	}
	body := `{"progress":1,"match_satisfaction":5,"idempotency_key":"consultation-feedback-001"}`
	if rr := call("POST", base+"/outcome", body, true, true); rr.Code != http.StatusConflict || stub.outcomeWrites != 0 {
		t.Fatalf("unverified completion accepted status=%d", rr.Code)
	}
	stub.completed = true
	if rr := call("POST", base+"/outcome", body, true, true); rr.Code != http.StatusOK || stub.outcomeWrites != 1 {
		t.Fatalf("verified outcome status=%d writes=%d body=%s", rr.Code, stub.outcomeWrites, rr.Body.String())
	}
	if rr := call("GET", base+"/outcome", "", true, false); rr.Code != http.StatusOK {
		t.Fatalf("outcome read status=%d", rr.Code)
	} else {
		var m map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil || m["progress"] != float64(1) {
			t.Fatalf("outcome=%s err=%v", rr.Body.String(), err)
		}
	}
	if rr := call("GET", "/v1/consultations/not-uuid/ready", "", true, false); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid UUID status=%d", rr.Code)
	}
}
