package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type accountPairTestRecord struct {
	codeDigest, proofDigest, id, actor string
	expires                            time.Time
	consumed                           bool
}
type accountPairTestStore struct {
	mu      sync.Mutex
	records map[string]*accountPairTestRecord
}

func (s *accountPairTestStore) CreateAccountDevicePair(id, codeDigest, proofDigest string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]*accountPairTestRecord)
	}
	s.records[id] = &accountPairTestRecord{id: id, codeDigest: codeDigest, proofDigest: proofDigest, expires: expires}
	return nil
}
func (s *accountPairTestStore) ApproveAccountDevicePair(codeDigest, actor string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.codeDigest == codeDigest && record.actor == "" && !record.consumed && time.Now().Before(record.expires) {
			record.actor = actor
			return true, nil
		}
	}
	return false, nil
}
func (s *accountPairTestStore) ClaimAccountDevicePair(id, proofDigest string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[id]
	if record == nil || record.proofDigest != proofDigest || record.actor == "" || record.consumed || !time.Now().Before(record.expires) {
		return "", false, nil
	}
	record.consumed = true
	return record.actor, true, nil
}

func TestAccountPairingAcrossDevicesKeepsCanonicalIdentity(t *testing.T) {
	key := []byte(strings.Repeat("S", 32))
	store := &accountPairTestStore{}
	h := New(Options{ClientSessionKey: key, AccountDevicePairing: store})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	const existing = "11111111-1111-4111-8111-111111111111"
	source, err := manager.issue(existing)
	if err != nil {
		t.Fatal(err)
	}
	post := func(path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "https://apgic.ru"+path, bytes.NewBufferString(body))
		req.Host = "apgic.ru"
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	const start = "/v1/account/devices/pair/start"
	const approve = "/v1/account/devices/pair/approve"
	const claim = "/v1/account/devices/pair/claim"
	if got := post(start, "", source, "https://apgic.ru").Code; got != 409 {
		t.Fatalf("existing target identity was replaceable: %d", got)
	}
	if got := post(start, "", nil, "https://evil.example").Code; got != 403 {
		t.Fatalf("cross-origin start allowed: %d", got)
	}
	started := post(start, "", nil, "https://apgic.ru")
	if started.Code != 201 {
		t.Fatalf("start status %d: %s", started.Code, started.Body)
	}
	var challenge struct {
		PairID string `json:"pair_id"`
		Code   string `json:"code"`
		Proof  string `json:"proof"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	if len(challenge.Code) != 16 || len(challenge.Proof) != 43 {
		t.Fatal("weak challenge")
	}
	claimBody, _ := json.Marshal(map[string]string{"pair_id": challenge.PairID, "proof": challenge.Proof})
	if got := post(claim, string(claimBody), nil, "https://apgic.ru").Code; got != 409 {
		t.Fatalf("unapproved device gained identity: %d", got)
	}
	approval, _ := json.Marshal(map[string]string{"code": challenge.Code})
	if got := post(approve, string(approval), nil, "https://apgic.ru").Code; got != 401 {
		t.Fatalf("unauthenticated approval: %d", got)
	}
	if got := post(approve, string(approval), source, "").Code; got != 403 {
		t.Fatalf("originless approval: %d", got)
	}
	if got := post(approve, string(approval), source, "https://apgic.ru").Code; got != 200 {
		t.Fatalf("trusted source approval: %d", got)
	}
	if got := post(claim, string(claimBody), source, "https://apgic.ru").Code; got != 409 {
		t.Fatalf("claim silently overwrote target account: %d", got)
	}
	claimed := post(claim, string(claimBody), nil, "https://apgic.ru")
	if claimed.Code != 200 {
		t.Fatalf("approved claim: %d: %s", claimed.Code, claimed.Body)
	}
	var target *http.Cookie
	for _, c := range claimed.Result().Cookies() {
		if c.Name == clientSessionCookieName {
			target = c
		}
	}
	if target == nil || !target.Secure || !target.HttpOnly || target.SameSite != http.SameSiteLaxMode {
		t.Fatal("target signed cookie missing or insecure")
	}
	req := httptest.NewRequest(http.MethodGet, "https://apgic.ru/v1/academy/my-courses", nil)
	req.AddCookie(target)
	actor, err := manager.identityFromRequest(req)
	if err != nil || actor != existing {
		t.Fatalf("device linked to wrong identity: %q, %v", actor, err)
	}
	if got := post(claim, string(claimBody), nil, "https://apgic.ru").Code; got != 409 {
		t.Fatalf("claim replay status %d", got)
	}
	if got := post(approve, string(approval), source, "https://apgic.ru").Code; got != 409 {
		t.Fatalf("approval replay status %d", got)
	}
	if got := post(claim, string(claimBody), nil, "https://evil.example").Code; got != 403 {
		t.Fatalf("cross-origin claim status %d", got)
	}
}
