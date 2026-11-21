package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyDisconnectTestStore struct {
	academyStore
	owner string
	calls int
}

func (s *academyDisconnectTestStore) AcademyDisconnectCloud(actor, provider string) error {
	if actor != s.owner || provider != "YANDEX_DISK" {
		return academy.ErrDenied
	}
	s.calls++
	return nil
}

func TestAcademyYandexDisconnectRequiresSignedAuthorAndExplicitOrigin(t *testing.T) {
	key := []byte(strings.Repeat("k", 40))
	sessions, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	const owner = "11111111-1111-4111-8111-111111111111"
	const outsider = "22222222-2222-4222-8222-222222222222"
	ownerCookie, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	outsiderCookie, err := sessions.issue(outsider)
	if err != nil {
		t.Fatal(err)
	}
	store := &academyDisconnectTestStore{owner: owner}
	handler := New(Options{ClientSessionKey: key, Academy: store})
	const route = "https://apgic.ru/v1/academy/author/cloud/yandex/disconnect"
	const confirmed = `{"confirm":"DISCONNECT_YANDEX_DISK"}`
	cases := []struct {
		name, origin, fetchSite, body string
		cookie                        *http.Cookie
		want                          int
		calls                         int
	}{
		{"anonymous", "https://apgic.ru", "", confirmed, nil, 401, 0},
		{"missing origin", "", "", confirmed, ownerCookie, 403, 0},
		{"cross-site", "https://attacker.invalid", "cross-site", confirmed, ownerCookie, 403, 0},
		{"cross-site metadata", "https://apgic.ru", "cross-site", confirmed, ownerCookie, 403, 0},
		{"missing confirmation", "https://apgic.ru", "same-origin", "{}", ownerCookie, 400, 0},
		{"forged confirmation", "https://apgic.ru", "same-origin", `{"confirm":"YES"}`, ownerCookie, 400, 0},
		{"foreign author", "https://apgic.ru", "same-origin", confirmed, outsiderCookie, 403, 0},
		{"valid first party owner", "https://apgic.ru", "same-origin", confirmed, ownerCookie, 200, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store.calls = 0
			req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(tc.body))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want || store.calls != tc.calls {
				t.Fatalf("status=%d calls=%d want %d/%d body=%s", rec.Code, store.calls, tc.want, tc.calls, rec.Body.String())
			}
			if tc.want == 200 && (!strings.Contains(rec.Body.String(), `"disconnected":true`) || rec.Header().Get("Cache-Control") != "private, no-store") {
				t.Fatalf("successful disconnect must confirm without leaking data: %s", rec.Body.String())
			}
		})
	}
}
