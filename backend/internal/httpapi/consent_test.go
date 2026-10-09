package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

func testGrowthConsentPolicies(t *testing.T) map[string]privacy.ConsentPolicy {
	t.Helper()
	policy, err := privacy.NewConsentPolicy(
		privacy.PurposeGrowthSessionProjection,
		"growth-v1",
		"sha256:text",
	)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]privacy.ConsentPolicy{policy.Purpose: policy}
}

type memoryConsentStore struct {
	records map[string]privacy.ConsentRecord
}

func (s *memoryConsentStore) RecordConsent(record privacy.ConsentRecord) (privacy.ConsentRecord, bool, error) {
	if s.records == nil {
		s.records = map[string]privacy.ConsentRecord{}
	}
	for _, existing := range s.records {
		if existing.SubjectID == record.SubjectID && existing.Purpose == record.Purpose &&
			existing.Scope == record.Scope && existing.RevokedAt == nil {
			return existing, true, nil
		}
	}
	s.records[record.ID] = record
	return record, false, nil
}

func (s *memoryConsentStore) RevokeConsent(consentID, subjectID string, revokedAt time.Time) (privacy.ConsentRecord, bool, error) {
	record, ok := s.records[consentID]
	if !ok || record.SubjectID != subjectID {
		return privacy.ConsentRecord{}, false, privacy.ErrConsentNotFound
	}
	if record.RevokedAt != nil {
		return record, true, nil
	}
	value := revokedAt.UTC()
	record.RevokedAt = &value
	s.records[consentID] = record
	return record, false, nil
}

func (s *memoryConsentStore) ActiveConsent(subjectID, purpose, scope string, at time.Time) (privacy.ConsentRecord, bool, error) {
	for _, record := range s.records {
		if record.SubjectID == subjectID && record.Purpose == purpose && record.Scope == scope &&
			record.RevokedAt == nil && !record.GrantedAt.After(at) {
			return record, true, nil
		}
	}
	return privacy.ConsentRecord{}, false, nil
}

func TestGrowthExportIgnoresClientAssertedConsentWithoutLedgerEvidence(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	ownerID := "11111111-1111-4111-8111-111111111111"
	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntentForIdentity(ownerID, "нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}

	store := &memoryConsentStore{}
	handler := New(Options{Demand: service, Consents: store, ConsentPolicies: testGrowthConsentPolicies(t), ClientSessionKey: key})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/consultations/"+hold.BookingID+"/growth-export",
		strings.NewReader(`{"purpose_consent":true}`),
	)
	request.AddCookie(cookie)
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("self-asserted consent status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "DATA_PURPOSE_CONSENT_REQUIRED") {
		t.Fatalf("consent ledger denial reason missing: %s", recorder.Body.String())
	}

	// A durable consent for an older policy version must not silently authorize
	// processing after the active policy rotates.
	store.records = map[string]privacy.ConsentRecord{
		"old-consent": {
			ID:                "old-consent",
			SubjectID:         ownerID,
			Purpose:           privacy.PurposeGrowthSessionProjection,
			Scope:             privacy.GrowthConsentScope(hold.BookingID),
			PolicyVersion:     "growth-old",
			TextHashOrVersion: "sha256:old",
			GrantedAt:         time.Now().UTC().Add(-time.Minute),
			Source:            "TEST",
			ProofMetadata:     []byte(`{"proof":"old"}`),
		},
	}
	rotated := httptest.NewRequest(http.MethodPost, "/v1/consultations/"+hold.BookingID+"/growth-export", nil)
	rotated.AddCookie(cookie)
	rotatedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(rotatedRecorder, rotated)
	if rotatedRecorder.Code != http.StatusConflict ||
		!strings.Contains(rotatedRecorder.Body.String(), "DATA_PURPOSE_CONSENT_REQUIRED") {
		t.Fatalf("stale durable consent authorized growth: status=%d body=%s", rotatedRecorder.Code, rotatedRecorder.Body.String())
	}
}

func TestConsentEndpointBindsSubjectAndRevocationToTrustedSession(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	subjectID := "33333333-3333-4333-8333-333333333333"
	now := time.Date(2030, 2, 3, 4, 5, 6, 0, time.UTC)
	store := &memoryConsentStore{}
	handler := New(Options{Consents: store, ConsentPolicies: testGrowthConsentPolicies(t), ClientSessionKey: key, Now: func() time.Time { return now }})
	manager, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue(subjectID)
	if err != nil {
		t.Fatal(err)
	}
	grant := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(
		`{"purpose":"GROWTH_SESSION_PROJECTION","scope":"booking/b1","policy_version":"growth-v1","text_hash_or_version":"sha256:text"}`,
	))
	grant.AddCookie(cookie)
	grant.Header.Set("content-type", "application/json")
	granted := httptest.NewRecorder()
	handler.ServeHTTP(granted, grant)
	if granted.Code != http.StatusCreated {
		t.Fatalf("grant status=%d body=%s", granted.Code, granted.Body.String())
	}
	var consentID string
	for id, record := range store.records {
		if record.SubjectID != subjectID {
			t.Fatalf("consent subject=%s want %s", record.SubjectID, subjectID)
		}
		consentID = id
	}
	if consentID == "" {
		t.Fatal("consent was not persisted")
	}

	revoke := httptest.NewRequest(http.MethodPost, "/v1/consents/"+consentID+"/revoke", nil)
	revoke.AddCookie(cookie)
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, revoke)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	if _, active, err := store.ActiveConsent(subjectID, privacy.PurposeGrowthSessionProjection, "booking/b1", now.Add(time.Second)); err != nil || active {
		t.Fatalf("revoked consent active=%v err=%v", active, err)
	}

	other, err := manager.issue("44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewRequest(http.MethodPost, "/v1/consents/"+consentID+"/revoke", nil)
	foreign.AddCookie(other)
	foreignRecorder := httptest.NewRecorder()
	handler.ServeHTTP(foreignRecorder, foreign)
	if foreignRecorder.Code != http.StatusNotFound && foreignRecorder.Code != http.StatusOK {
		t.Fatalf("foreign revoke status=%d body=%s", foreignRecorder.Code, foreignRecorder.Body.String())
	}
	if foreignRecorder.Code == http.StatusOK {
		t.Fatal(errors.New("foreign subject unexpectedly revoked consent"))
	}
}

func TestConsentEndpointRejectsStalePolicyVersion(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	now := time.Date(2030, 2, 3, 4, 5, 6, 0, time.UTC)
	store := &memoryConsentStore{}
	handler := New(Options{
		Consents:         store,
		ConsentPolicies:  testGrowthConsentPolicies(t),
		ClientSessionKey: key,
		Now:              func() time.Time { return now },
	})
	manager, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("55555555-5555-4555-8555-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(
		`{"purpose":"GROWTH_SESSION_PROJECTION","scope":"booking/b1","policy_version":"growth-old","text_hash_or_version":"sha256:old"}`,
	))
	request.AddCookie(cookie)
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale consent status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "CONSENT_POLICY_VERSION_MISMATCH") {
		t.Fatalf("stale consent reason missing: %s", recorder.Body.String())
	}
	if len(store.records) != 0 {
		t.Fatalf("stale policy consent was persisted: %#v", store.records)
	}
}
