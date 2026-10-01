package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

type deepLinkTestStore struct {
	resources map[string]mobile.DeepLinkResource
}

type failOnDeepLinkLookupStore struct{}

func (failOnDeepLinkLookupStore) DeepLinkResource(mobile.LinkKind, string) (mobile.DeepLinkResource, bool, error) {
	panic("protected resource lookup occurred before authentication")
}

func (s deepLinkTestStore) DeepLinkResource(kind mobile.LinkKind, targetID string) (mobile.DeepLinkResource, bool, error) {
	value, ok := s.resources[string(kind)+":"+targetID]
	return value, ok, nil
}

func TestMobileDeepLinkHTTPUsesTrustedSessionAndCurrentResourceTruth(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sessionKey := []byte(strings.Repeat("s", 32))
	sessions, err := newClientSessionManager(sessionKey, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("identity-a")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie, err := sessions.issue("identity-b")
	if err != nil {
		t.Fatal(err)
	}
	targetID := "00000000-0000-0000-0000-000000000711"
	store := deepLinkTestStore{resources: map[string]mobile.DeepLinkResource{
		"BOOKING:" + targetID: {
			Kind:              mobile.LinkBooking,
			TargetID:          targetID,
			AccessClass:       mobile.LinkProtectedResource,
			TenantID:          "tenant-booking",
			SubjectIdentityID: "identity-a",
		},
	}}
	handler := New(Options{
		DeepLinks:          store,
		DeepLinkSigningKey: []byte(strings.Repeat("d", 32)),
		ClientSessionKey:   sessionKey,
		Now:                func() time.Time { return now },
	})

	body, _ := json.Marshal(issueDeepLinkRequest{Kind: mobile.LinkBooking, TargetID: targetID})
	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/deep-links", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var issued issueDeepLinkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" || !strings.HasPrefix(issued.UniversalURL, "https://apgic.ru/l/") {
		t.Fatalf("unexpected issued link: %#v", issued)
	}

	resolve := httptest.NewRequest(http.MethodGet, "/v1/mobile/deep-links/resolve?token="+issued.Token, nil)
	resolve.AddCookie(cookie)
	resolved := httptest.NewRecorder()
	handler.ServeHTTP(resolved, resolve)
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", resolved.Code, resolved.Body.String())
	}
	var resolution deepLinkResolutionResponse
	if err := json.Unmarshal(resolved.Body.Bytes(), &resolution); err != nil {
		t.Fatal(err)
	}
	if resolution.Decision != "ALLOW" || resolution.CanonicalPath != "/bookings/"+targetID {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}

	crossUser := httptest.NewRequest(http.MethodGet, "/v1/mobile/deep-links/resolve?token="+issued.Token, nil)
	crossUser.AddCookie(otherCookie)
	crossUserRec := httptest.NewRecorder()
	handler.ServeHTTP(crossUserRec, crossUser)
	if crossUserRec.Code != http.StatusOK {
		t.Fatalf("cross-user resolution status=%d body=%s", crossUserRec.Code, crossUserRec.Body.String())
	}
	if err := json.Unmarshal(crossUserRec.Body.Bytes(), &resolution); err != nil {
		t.Fatal(err)
	}
	if resolution.Decision != "DENY" || !strings.HasPrefix(resolution.ReasonCode, mobile.ReasonLinkSubjectMismatch) {
		t.Fatalf("cross-user token must fail closed: %#v", resolution)
	}

	changed := store.resources["BOOKING:"+targetID]
	changed.SubjectIdentityID = "identity-b"
	store.resources["BOOKING:"+targetID] = changed
	changedHandler := New(Options{
		DeepLinks:          store,
		DeepLinkSigningKey: []byte(strings.Repeat("d", 32)),
		ClientSessionKey:   sessionKey,
		Now:                func() time.Time { return now },
	})
	stale := httptest.NewRequest(http.MethodGet, "/v1/mobile/deep-links/resolve?token="+issued.Token, nil)
	stale.AddCookie(cookie)
	staleRec := httptest.NewRecorder()
	changedHandler.ServeHTTP(staleRec, stale)
	if err := json.Unmarshal(staleRec.Body.Bytes(), &resolution); err != nil {
		t.Fatal(err)
	}
	if resolution.Decision != "DENY" || resolution.ReasonCode != mobile.ReasonLinkInvalid {
		t.Fatalf("stale ownership token must be invalidated: %#v", resolution)
	}
}

func TestMobileDeepLinkProtectedLookupRequiresSessionFirst(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	signingKey := []byte(strings.Repeat("d", 32))
	sessionKey := []byte(strings.Repeat("s", 32))
	handler := New(Options{
		DeepLinks:          failOnDeepLinkLookupStore{},
		DeepLinkSigningKey: signingKey,
		ClientSessionKey:   sessionKey,
		Now:                func() time.Time { return now },
	})

	targetID := "00000000-0000-0000-0000-000000000799"
	body, _ := json.Marshal(issueDeepLinkRequest{Kind: mobile.LinkBooking, TargetID: targetID})
	issue := httptest.NewRequest(http.MethodPost, "/v1/mobile/deep-links", bytes.NewReader(body))
	issueRec := httptest.NewRecorder()
	handler.ServeHTTP(issueRec, issue)
	if issueRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated protected issue status=%d body=%s", issueRec.Code, issueRec.Body.String())
	}

	manager, err := mobile.NewDeepLinkTokenManager(signingKey)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Issue(mobile.DeepLinkResource{
		Kind:              mobile.LinkBooking,
		TargetID:          targetID,
		AccessClass:       mobile.LinkProtectedResource,
		TenantID:          "tenant-a",
		SubjectIdentityID: "identity-a",
	}, "link-preauth", now, now.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	resolve := httptest.NewRequest(http.MethodGet, "/v1/mobile/deep-links/resolve?token="+token, nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolve)
	if resolveRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated protected resolve status=%d body=%s", resolveRec.Code, resolveRec.Body.String())
	}
}

func TestMobileDeepLinkIssueRejectsCrossUser(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sessionKey := []byte(strings.Repeat("s", 32))
	sessions, _ := newClientSessionManager(sessionKey, func() time.Time { return now })
	otherCookie, _ := sessions.issue("identity-b")
	targetID := "00000000-0000-0000-0000-000000000712"
	handler := New(Options{
		DeepLinks: deepLinkTestStore{resources: map[string]mobile.DeepLinkResource{
			"NOTIFICATION:" + targetID: {
				Kind:              mobile.LinkNotification,
				TargetID:          targetID,
				AccessClass:       mobile.LinkProtectedResource,
				TenantID:          "tenant-notification",
				SubjectIdentityID: "identity-a",
			},
		}},
		DeepLinkSigningKey: []byte(strings.Repeat("d", 32)),
		ClientSessionKey:   sessionKey,
		Now:                func() time.Time { return now },
	})
	body, _ := json.Marshal(issueDeepLinkRequest{Kind: mobile.LinkNotification, TargetID: targetID})
	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/deep-links", bytes.NewReader(body))
	req.AddCookie(otherCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-user issue status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMobileDeepLinkIssueRejectsInvalidLookupBeforeStore(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	handler := New(Options{
		DeepLinks:          deepLinkTestStore{resources: map[string]mobile.DeepLinkResource{}},
		DeepLinkSigningKey: []byte(strings.Repeat("d", 32)),
		ClientSessionKey:   []byte(strings.Repeat("s", 32)),
		Now:                func() time.Time { return now },
	})

	for name, body := range map[string]string{
		"null":         `null`,
		"empty object": `{}`,
		"unknown kind": `{"kind":"UNKNOWN","target_id":"resource-1"}`,
		"blank target": `{"kind":"SPECIALIST","target_id":"   "}`,
		"path smuggle": `{"kind":"SPECIALIST","target_id":"../admin"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/mobile/deep-links", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestMobileDeepLinkIssueRejectsTrailingOrOversizedJSON(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	handler := New(Options{
		DeepLinks:          deepLinkTestStore{resources: map[string]mobile.DeepLinkResource{}},
		DeepLinkSigningKey: []byte(strings.Repeat("d", 32)),
		ClientSessionKey:   []byte(strings.Repeat("s", 32)),
		Now:                func() time.Time { return now },
	})

	for name, body := range map[string]string{
		"trailing object": `{"kind":"SPECIALIST","target_id":"x"}{"kind":"SPECIALIST","target_id":"y"}`,
		"oversized":       `{"kind":"SPECIALIST","target_id":"` + strings.Repeat("x", maxDeepLinkIssueBodySize) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/mobile/deep-links", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
