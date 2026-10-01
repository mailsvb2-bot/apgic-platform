package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
)

type memoryMutationRecord struct {
	envelope      mutation.Envelope
	mutationID    string
	state         string
	sideEffectRef string
	failureCode   string
}

type memoryMutationStore struct {
	mu      sync.Mutex
	records map[string]memoryMutationRecord
}

func newMemoryMutationStore() *memoryMutationStore {
	return &memoryMutationStore{records: make(map[string]memoryMutationRecord)}
}

func (s *memoryMutationStore) key(envelope mutation.Envelope) string {
	return envelope.IdentityID + "|" + envelope.Operation + "|" + envelope.IdempotencyKey
}

func (s *memoryMutationStore) Claim(_ context.Context, mutationID string, envelope mutation.Envelope, _ time.Time) (mutation.ClaimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.key(envelope)
	if existing, ok := s.records[key]; ok {
		if existing.envelope.RequestDigest != envelope.RequestDigest {
			return mutation.ClaimResult{
				Outcome:    mutation.OutcomeConflict,
				MutationID: existing.mutationID,
				State:      existing.state,
			}, nil
		}
		outcome := mutation.OutcomeDuplicate
		if existing.state == mutation.StateApplied {
			outcome = mutation.OutcomeDuplicateApplied
		}
		if existing.state == mutation.StateFailed {
			outcome = mutation.OutcomeFailed
		}
		return mutation.ClaimResult{
			Outcome:       outcome,
			MutationID:    existing.mutationID,
			State:         existing.state,
			SideEffectRef: existing.sideEffectRef,
			FailureCode:   existing.failureCode,
		}, nil
	}
	s.records[key] = memoryMutationRecord{
		envelope:   envelope,
		mutationID: mutationID,
		state:      mutation.StateClaimed,
	}
	return mutation.ClaimResult{
		Outcome:    mutation.OutcomeClaimed,
		MutationID: mutationID,
		State:      mutation.StateClaimed,
	}, nil
}

func (s *memoryMutationStore) Finalize(_ context.Context, mutationID, sideEffectRef string, _ time.Time) (mutation.FinalizeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, record := range s.records {
		if record.mutationID != mutationID {
			continue
		}
		if record.state == mutation.StateApplied {
			if record.sideEffectRef == sideEffectRef {
				return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_APPLIED"}, nil
			}
			return mutation.FinalizeResult{ReasonCode: "MUTATION_SIDE_EFFECT_CONFLICT"}, nil
		}
		if record.state == mutation.StateFailed {
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_FAILED"}, nil
		}
		record.state = mutation.StateApplied
		record.sideEffectRef = sideEffectRef
		s.records[key] = record
		return mutation.FinalizeResult{Changed: true, ReasonCode: "MUTATION_APPLIED"}, nil
	}
	return mutation.FinalizeResult{ReasonCode: "MUTATION_FINALIZE_INVALID"}, nil
}

func (s *memoryMutationStore) Fail(_ context.Context, mutationID, failureCode string, _ time.Time) (mutation.FinalizeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, record := range s.records {
		if record.mutationID != mutationID {
			continue
		}
		if record.state == mutation.StateApplied {
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_APPLIED"}, nil
		}
		if record.state == mutation.StateFailed {
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_FAILED"}, nil
		}
		record.state = mutation.StateFailed
		record.failureCode = failureCode
		s.records[key] = record
		return mutation.FinalizeResult{Changed: true, ReasonCode: "MUTATION_FAILED"}, nil
	}
	return mutation.FinalizeResult{ReasonCode: "MUTATION_FAIL_INVALID"}, nil
}

func TestMobileCheckoutMutationReplaysOneCanonicalCheckout(t *testing.T) {
	key := []byte(strings.Repeat("u", 32))
	service := demand.NewConformanceService(nil)
	store := newMemoryMutationStore()
	handler := New(Options{
		Demand:           service,
		ClientMutations:  store,
		ClientSessionKey: key,
	})

	call := func(cookie *http.Cookie, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("content-type", "application/json")
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	bootstrap := call(nil, http.MethodPost, "/v1/help-intents", `{"free_text":"sleep"}`, nil)
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	var intent demand.Intent
	if err := json.Unmarshal(bootstrap.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	var session *http.Cookie
	for _, candidate := range bootstrap.Result().Cookies() {
		if candidate.Name == "__Host-apgic_session" {
			session = candidate
			break
		}
	}
	if session == nil {
		t.Fatal("trusted client session missing")
	}

	confirm := call(session, http.MethodPost, "/v1/help-intents/"+intent.ID+"/confirm", `{"topics":["sleep"],"goals":[],"context":{}}`, nil)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirm.Code, confirm.Body.String())
	}

	slotsResponse := call(session, http.MethodGet, "/v1/specialists/spec-lebedeva/slots", "", nil)
	if slotsResponse.Code != http.StatusOK {
		t.Fatalf("slots status=%d body=%s", slotsResponse.Code, slotsResponse.Body.String())
	}
	var slots struct {
		Slots []demand.Slot `json:"slots"`
	}
	if err := json.Unmarshal(slotsResponse.Body.Bytes(), &slots); err != nil {
		t.Fatal(err)
	}
	if len(slots.Slots) == 0 {
		t.Fatal("conformance catalog returned no slots")
	}

	holdResponse := call(session, http.MethodPost, "/v1/slot-holds",
		`{"help_intent_id":"`+intent.ID+`","slot_id":"`+slots.Slots[0].ID+`"}`, nil)
	if holdResponse.Code != http.StatusCreated {
		t.Fatalf("hold status=%d body=%s", holdResponse.Code, holdResponse.Body.String())
	}
	var hold demand.Hold
	if err := json.Unmarshal(holdResponse.Body.Bytes(), &hold); err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{"Idempotency-Key": "mobile-checkout-retry-1"}
	body := `{"hold_id":"` + hold.ID + `","method_code":"BANK_CARD"}`
	first := call(session, http.MethodPost, "/v1/mobile/checkout-instructions", body, headers)
	if first.Code != http.StatusCreated {
		t.Fatalf("first checkout status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResult mobileCheckoutMutationResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	if firstResult.Outcome != "APPLIED" || firstResult.Checkout == nil || firstResult.SideEffectRef == "" {
		t.Fatalf("first result=%#v", firstResult)
	}

	retry := call(session, http.MethodPost, "/v1/mobile/checkout-instructions", body, headers)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry checkout status=%d body=%s", retry.Code, retry.Body.String())
	}
	var retryResult mobileCheckoutMutationResponse
	if err := json.Unmarshal(retry.Body.Bytes(), &retryResult); err != nil {
		t.Fatal(err)
	}
	if retryResult.Outcome != "DUPLICATE_APPLIED" ||
		retryResult.SideEffectRef != firstResult.SideEffectRef ||
		retryResult.Checkout == nil ||
		retryResult.Checkout.ID != firstResult.Checkout.ID ||
		retryResult.Checkout.OrderID != firstResult.Checkout.OrderID ||
		retryResult.Checkout.BookingID != firstResult.Checkout.BookingID {
		t.Fatalf("retry created divergent side effect: first=%#v retry=%#v", firstResult, retryResult)
	}

	conflict := call(session, http.MethodPost, "/v1/mobile/checkout-instructions",
		`{"hold_id":"`+hold.ID+`","method_code":"SBP"}`, headers)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "MUTATION_IDEMPOTENCY_CONFLICT") {
		t.Fatalf("changed payload status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestMobileCheckoutMutationPersistsTerminalFailure(t *testing.T) {
	key := []byte(strings.Repeat("v", 32))
	service := demand.NewConformanceService(nil)
	store := newMemoryMutationStore()
	handler := New(Options{
		Demand:           service,
		ClientMutations:  store,
		ClientSessionKey: key,
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", strings.NewReader(`{"free_text":"sleep"}`))
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var session *http.Cookie
	for _, candidate := range recorder.Result().Cookies() {
		if candidate.Name == "__Host-apgic_session" {
			session = candidate
			break
		}
	}
	if session == nil {
		t.Fatal("trusted client session missing")
	}

	callInvalid := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/mobile/checkout-instructions",
			strings.NewReader(`{"hold_id":"missing-hold","method_code":"NOT_A_METHOD"}`))
		req.Header.Set("content-type", "application/json")
		req.Header.Set("Idempotency-Key", "mobile-checkout-failure-1")
		req.AddCookie(session)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}
	first := callInvalid()
	if first.Code != http.StatusNotFound {
		t.Fatalf("first terminal failure status=%d body=%s", first.Code, first.Body.String())
	}
	replay := callInvalid()
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "BOOK_HOLD_NOT_FOUND") {
		t.Fatalf("replayed terminal failure status=%d body=%s", replay.Code, replay.Body.String())
	}
}
