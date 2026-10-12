package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

type verifiedFeedbackSpy struct {
	*specialistStoreSpy
	seenClient  string
	seenBooking string
	seenRating  int
}

func (s *verifiedFeedbackSpy) SubmitVerifiedFeedback(clientID, bookingID string, rating int) (specialist.FeedbackSubmission, error) {
	s.seenClient, s.seenBooking, s.seenRating = clientID, bookingID, rating
	if rating < 1 || rating > 5 {
		return specialist.FeedbackSubmission{}, specialist.ErrFeedbackInvalid
	}
	return specialist.FeedbackSubmission{ID: "00000000-0000-0000-0000-000000000001", State: "PENDING"}, nil
}
func (s *verifiedFeedbackSpy) ReviewVerifiedFeedback(string, bool, string) error { return nil }

func TestVerifiedFeedbackSubmissionRequiresSignedClientAndCannotClaimReview(t *testing.T) {
	const client = "11111111-1111-4111-8111-111111111111"
	const booking = "22222222-2222-4222-8222-222222222222"
	key := []byte(strings.Repeat("t", 32))
	manager, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue(client)
	if err != nil {
		t.Fatal(err)
	}
	spy := &verifiedFeedbackSpy{specialistStoreSpy: &specialistStoreSpy{}}
	handler := New(Options{Specialists: spy, ClientSessionKey: key})
	path := "/v1/specialist-feedback"
	post := func(body string, signed bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if signed {
			req.AddCookie(cookie)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	if got := post(`{"booking_id":"`+booking+`","rating":5}`, false); got.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous review accepted: %d", got.Code)
	}
	if got := post(`{"booking_id":"`+booking+`","rating":5,"state":"APPROVED"}`, true); got.Code != http.StatusBadRequest {
		t.Fatalf("client approved own rating: %d", got.Code)
	}
	if got := post(`{"booking_id":"`+booking+`","rating":6}`, true); got.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range rating accepted: %d", got.Code)
	}
	response := post(`{"booking_id":"`+booking+`","rating":5}`, true)
	if response.Code != http.StatusCreated || spy.seenClient != client || spy.seenBooking != booking || spy.seenRating != 5 {
		t.Fatalf("signed submission failed: code=%d client=%q booking=%q rating=%d", response.Code, spy.seenClient, spy.seenBooking, spy.seenRating)
	}
	if strings.Contains(response.Body.String(), "client_identity_id") || strings.Contains(response.Body.String(), "booking_id") {
		t.Fatalf("feedback response leaked owner: %s", response.Body.String())
	}
}
