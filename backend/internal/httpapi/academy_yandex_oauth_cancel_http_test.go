package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAcademyYandexCancelledConsentHTTPReturnsToEditor(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	t.Setenv("APGIC_ACADEMY_OAUTH_STATE_KEY", base64.StdEncoding.EncodeToString(key))
	sessions, err := newClientSessionManager([]byte(strings.Repeat("k", 32)), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	store := &academyStoreStub{}
	mux := http.NewServeMux()
	registerAcademyYandexOAuth(mux, store, sessions, nil)
	const course = "11111111-1111-4111-8111-111111111111"
	const version = "22222222-2222-4222-8222-222222222222"
	const lesson = "33333333-3333-4333-8333-333333333333"
	nonce := strings.Repeat("n", 32)
	sealed, err := cloudSeal(cloudOAuthState{Actor: "author", Verifier: strings.Repeat("v", 43), Nonce: nonce,
		ReturnCourse: course, ReturnVersion: version, ReturnLesson: lesson,
		Expires: time.Now().Add(time.Minute).Unix()}, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query, want string
		cookie            bool
	}{
		{"matching state", "error=access_denied&state=" + nonce,
			"/academy?cloud_error=denied&course_id=" + course + "&lesson_id=" + lesson + "&version_id=" + version, true},
		{"missing cookie", "error=access_denied&state=" + nonce, "/academy?cloud_error=denied", false},
		{"unmatched state", "error=access_denied&state=" + strings.Repeat("x", 32), "/academy?cloud_error=denied", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/academy/author/cloud/yandex/callback?"+tc.query, nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "apgic_academy_yandex_oauth", Value: sealed})
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.want {
				t.Fatalf("status=%d location=%q want=%q", rec.Code, rec.Header().Get("Location"), tc.want)
			}
		})
	}
	if store.calls != 0 {
		t.Fatalf("OAuth cancellation reached credential storage %d times", store.calls)
	}
}
