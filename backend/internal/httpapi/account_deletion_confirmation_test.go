package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type deletionConfirmationFixture struct {
	calls      int
	owner      string
	sessionRef string
	active     bool
}

func (s *deletionConfirmationFixture) SubmitAccountDeletionRequest(_ context.Context, _, _ string) (privacy.AccountDeletionReceipt, error) {
	return privacy.AccountDeletionReceipt{ID: "11111111-1111-4111-8111-111111111111", State: "REQUESTED"}, nil
}

func (s *deletionConfirmationFixture) ReadAccountDeletionRequest(_ context.Context, _ string) (privacy.AccountDeletionReceipt, error) {
	return privacy.AccountDeletionReceipt{ID: "11111111-1111-4111-8111-111111111111", State: "REQUESTED"}, nil
}

func (s *deletionConfirmationFixture) DeletionFactorActiveAt(_ context.Context, _ string, _ time.Time) (bool, error) {
	return s.active, nil
}

func (s *deletionConfirmationFixture) ReconfirmAccountDeletionRequest(_ context.Context, owner, sessionRef string, _ time.Time) (privacy.AccountDeletionReceipt, error) {
	s.calls++
	s.owner = owner
	s.sessionRef = sessionRef
	return privacy.AccountDeletionReceipt{ID: "11111111-1111-4111-8111-111111111111", State: "IDENTITY_RECONFIRMED", Notice: "Стирание не выполнено"}, nil
}

func (s *deletionConfirmationFixture) ConfirmAccountDeletionFromSession(_ context.Context, owner, sessionRef string) (privacy.AccountDeletionReceipt, error) {
	s.calls++
	s.owner = owner
	s.sessionRef = sessionRef
	return privacy.AccountDeletionReceipt{ID:"11111111-1111-4111-8111-111111111111", State:"IDENTITY_RECONFIRMED", Notice:"Удаление ещё не выполнено"}, nil
}

func TestExplicitSessionDeletionConfirmationRequiresOwnerAndAffirmation(t *testing.T) {
	const owner = "22222222-2222-4222-8222-222222222222"
	key := []byte(strings.Repeat("k",48))
	store := &deletionConfirmationFixture{}
	now := time.Now().UTC()
	handler := New(Options{ClientSessionKey:key, AccountDeletionIntake:store, Now:func()time.Time{return now}})
	sessions,err := newClientSessionManager(key,func()time.Time{return now})
	if err!=nil {t.Fatal(err)}
	signed,err := sessions.issue(owner)
	if err!=nil {t.Fatal(err)}
	sessionRequest:=httptest.NewRequest(http.MethodGet,"/",nil)
	sessionRequest.AddCookie(signed)
	_,sessionRef,err:=sessions.identityAndReferenceFromRequest(sessionRequest)
	if err!=nil {t.Fatal(err)}
	post:=func(cookie *http.Cookie, body, header, origin string)*httptest.ResponseRecorder{
		q:=httptest.NewRequest(http.MethodPost,"https://apgic.ru/v1/client/account-deletion-requests/confirm",strings.NewReader(body))
		q.Header.Set("Content-Type","application/json")
		q.Header.Set("X-APGIC-Deletion-Confirmation",header)
		if origin!="" {q.Header.Set("Origin",origin)}
		if cookie!=nil {q.AddCookie(cookie)}
		w:=httptest.NewRecorder()
		handler.ServeHTTP(w,q)
		return w
	}
	if w:=post(nil,`{"confirm":true}`,"explicit-session-v1","https://apgic.ru");w.Code!=http.StatusUnauthorized {t.Fatalf("anonymous deletion allowed: %d",w.Code)}
	if w:=post(signed,`{"confirm":false}`,"explicit-session-v1","https://apgic.ru");w.Code!=http.StatusBadRequest {t.Fatalf("implicit deletion allowed: %d",w.Code)}
	if w:=post(signed,`{"confirm":true}`,"", "https://apgic.ru");w.Code!=http.StatusUnsupportedMediaType {t.Fatalf("missing confirmation marker accepted: %d",w.Code)}
	if w:=post(signed,`{"confirm":true}`,"explicit-session-v1","https://attacker.example");w.Code!=http.StatusForbidden {t.Fatalf("foreign origin accepted: %d",w.Code)}
	if w:=post(signed,`{"confirm":true}`,"explicit-session-v1","https://apgic.ru");w.Code!=http.StatusAccepted || !strings.Contains(w.Body.String(),"IDENTITY_RECONFIRMED") {t.Fatalf("explicit confirmation rejected: %d %s",w.Code,w.Body.String())}
	if store.calls!=1 || store.owner!=owner || store.sessionRef!=sessionRef {t.Fatalf("wrong owner/session confirmation: %+v",store)}
}

func TestDeletionReconfirmationRequiresActiveSessionBoundVerifiedFactor(t *testing.T) {
	const owner = "22222222-2222-4222-8222-222222222222"
	key := []byte(strings.Repeat("k", 48))
	store := &deletionConfirmationFixture{active: true}
	now := time.Now().UTC()
	handler := New(Options{ClientSessionKey: key, AccountDeletionIntake: store, Now: func() time.Time { return now }})
	sessions, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	stepUp, err := newStepUpManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	sessionRequest.AddCookie(signed)
	_, sessionRef, err := sessions.identityAndReferenceFromRequest(sessionRequest)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := stepUp.issue(owner, sessionRef, "DELETION_TOTP", now)
	if err != nil {
		t.Fatal(err)
	}
	request := func(cookie *http.Cookie, proof *http.Cookie) *httptest.ResponseRecorder {
		q := httptest.NewRequest(http.MethodPost, "/v1/client/account-deletion-requests/reconfirm", strings.NewReader(`{"confirm":true}`))
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set("X-APGIC-Deletion-Confirmation", "verified-step-up-v1")
		if cookie != nil {
			q.AddCookie(cookie)
		}
		if proof != nil {
			q.AddCookie(proof)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, q)
		return w
	}
	if w := request(nil, verified); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned confirmation accepted: %d", w.Code)
	}
	if w := request(signed, nil); w.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing independent factor accepted: %d", w.Code)
	}
	otherSession, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	if w := request(otherSession, verified); w.Code != http.StatusPreconditionRequired {
		t.Fatalf("factor transferred across sessions: %d", w.Code)
	}
	store.active = false
	if w := request(signed, verified); w.Code != http.StatusForbidden {
		t.Fatalf("revoked factor accepted: %d", w.Code)
	}
	store.active = true
	if w := request(signed, verified); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "IDENTITY_RECONFIRMED") {
		t.Fatalf("verified confirmation rejected: %d %s", w.Code, w.Body.String())
	}
	if store.calls != 1 || store.owner != owner || store.sessionRef != sessionRef {
		t.Fatalf("unexpected transition identity: %+v", store)
	}
}
