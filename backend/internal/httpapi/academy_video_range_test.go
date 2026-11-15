package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcademyVideoSingleRange(t *testing.T) {
	for _, value := range []string{"bytes=0-", "bytes=0-1023", "bytes=-500"} {
		if !academyVideoSingleRange(value) {
			t.Fatalf("valid range rejected: %q", value)
		}
	}
	for _, value := range []string{"bytes=", "bytes=--", "bytes=1-2,3-4", "bytes=x-y", "bytes= 1-", "items=0-2", "bytes=0-2\r\nX-Evil: 1", "bytes=+1-", "bytes=1-+2", "bytes=-+5", "bytes=1 -2", "bytes=1- 2", "bytes=-0", "bytes=10-9", "bytes=18446744073709551616-", "bytes=0-18446744073709551616"} {
		if academyVideoSingleRange(value) {
			t.Fatalf("invalid range accepted: %q", value)
		}
	}
}

func TestAcademyVideoRequestedRangeRejectsDuplicateAndMalformedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers http.Header
		want    string
		ok      bool
	}{
		{"absent", http.Header{}, "", true},
		{"valid seek", http.Header{"Range": {"bytes=100-200"}}, "bytes=100-200", true},
		{"valid suffix", http.Header{"Range": {"bytes=-100"}}, "bytes=-100", true},
		{"duplicate conflicting", http.Header{"Range": {"bytes=0-99", "bytes=100-199"}}, "", false},
		{"duplicate identical", http.Header{"Range": {"bytes=0-99", "bytes=0-99"}}, "", false},
		{"combined ranges", http.Header{"Range": {"bytes=0-99,100-199"}}, "", false},
		{"zero suffix", http.Header{"Range": {"bytes=-0"}}, "", false},
		{"reversed", http.Header{"Range": {"bytes=99-1"}}, "", false},
		{"overflow", http.Header{"Range": {"bytes=18446744073709551616-"}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := academyVideoRequestedRange(tc.headers)
			if value != tc.want || ok != tc.ok {
				t.Fatalf("range=(%q,%v), want (%q,%v)", value, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAcademyVideoStreamRejectsMalformedRangeAsBadRequestBeforeProviderLookup(t *testing.T) {
	key := []byte(strings.Repeat("k", 48))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("identity-cryptographically-signed")
	if err != nil {
		t.Fatal(err)
	}
	store := &academyStoreStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	request := httptest.NewRequest(http.MethodGet,
		"/v1/academy/enrollments/00000000-0000-4000-8000-000000000001/lessons/00000000-0000-4000-8000-000000000002/videos/00000000-0000-4000-8000-000000000003/stream",
		nil)
	request.AddCookie(cookie)
	request.Header.Add("Range", "bytes=0-99,100-199")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed multi-range returned %d, want 400", response.Code)
	}
	if response.Header().Get("Content-Range") != "" {
		t.Fatal("malformed range was misrepresented as an unsatisfied representation range")
	}
	if store.calls != 0 {
		t.Fatalf("malformed range reached Academy/provider storage %d times", store.calls)
	}
}
