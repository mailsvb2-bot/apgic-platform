package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
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
	mu       sync.Mutex
	records  map[string]*accountPairTestRecord
	sessions map[string]*accountPairTestSession
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

type accountPairTestSession struct {
	id, actor, digest string
	expires           time.Time
	revoked           bool
}

func (s *accountPairTestStore) RegisterAccountPairedSession(id, actor, digest string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]*accountPairTestSession)
	}
	s.sessions[id] = &accountPairTestSession{id: id, actor: actor, digest: digest, expires: expires}
	return nil
}
func (s *accountPairTestStore) AccountPairedSessionActive(actor, digest string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if session.actor == actor && session.digest == digest && !session.revoked && time.Now().Before(session.expires) {
			return true, nil
		}
	}
	return false, nil
}
func (s *accountPairTestStore) ListAccountPairedSessions(actor string) ([]identity.PairedDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]identity.PairedDevice, 0)
	for _, session := range s.sessions {
		if session.actor == actor && !session.revoked && time.Now().Before(session.expires) {
			result = append(result, identity.PairedDevice{ID: session.id, CreatedAt: time.Now(), ExpiresAt: session.expires})
		}
	}
	return result, nil
}
func (s *accountPairTestStore) RevokeAccountPairedSession(actor, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil || session.actor != actor || session.revoked {
		return false, nil
	}
	session.revoked = true
	return true, nil
}

func TestAccountPairingAcrossDevicesKeepsCanonicalIdentity(t *testing.T) {
	key := []byte(strings.Repeat("S", 32))
	store := &accountPairTestStore{}
	h := New(Options{ClientSessionKey: key, AccountDevicePairing: store})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.pairedSessions = store
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
	// Next.js forwards the browser Origin but changes Host to its loopback API.
	proxied:=httptest.NewRequest(http.MethodPost,"http://127.0.0.1:43111"+start,nil)
	proxied.Host="127.0.0.1:43111"
	proxied.Header.Set("Origin","https://apgic.ru")
	proxyResponse:=httptest.NewRecorder()
	h.ServeHTTP(proxyResponse,proxied)
	if proxyResponse.Code!=201 {t.Fatalf("valid origin behind Next proxy denied: %d %s",proxyResponse.Code,proxyResponse.Body.String())}
	www:=httptest.NewRequest(http.MethodPost,"http://127.0.0.1:43111"+start,nil)
	www.Host="127.0.0.1:43111"
	www.Header.Set("Origin","https://www.apgic.ru")
	wwwResult:=httptest.NewRecorder();h.ServeHTTP(wwwResult,www)
	if wwwResult.Code!=201 {t.Fatalf("www first-party origin denied: %d",wwwResult.Code)}
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
	listRequest := httptest.NewRequest(http.MethodGet, "https://apgic.ru/v1/account/devices", nil)
	listRequest.AddCookie(source)
	listResult := httptest.NewRecorder()
	h.ServeHTTP(listResult, listRequest)
	if listResult.Code != 200 {
		t.Fatalf("device list status %d", listResult.Code)
	}
	var listing struct {
		Devices []identity.PairedDevice `json:"devices"`
	}
	if err := json.Unmarshal(listResult.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Devices) != 1 {
		t.Fatalf("device listing count %d", len(listing.Devices))
	}
	if got := post("/v1/account/devices/"+listing.Devices[0].ID+"/revoke", "", nil, "https://apgic.ru").Code; got != 401 {
		t.Fatalf("unauthenticated device revoke status %d", got)
	}
	if got := post("/v1/account/devices/"+listing.Devices[0].ID+"/revoke", "", source, "https://apgic.ru").Code; got != 200 {
		t.Fatalf("owner device revoke status %d", got)
	}
	if _, err := manager.identityFromRequest(req); !errors.Is(err, ErrClientSessionInvalid) {
		t.Fatalf("revoked paired v3 session was still valid: %v", err)
	}
}
