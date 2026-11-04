package httpapi

import (
	"encoding/json"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"net/http/httptest"
	"testing"
)

func TestAcademyCloudHTTPErrorIsActionableJSON(t *testing.T) {
	cases := map[int]struct {
		status int
		code   string
	}{
		401: {409, "ACADEMY_CLOUD_REAUTH_REQUIRED"},
		404: {404, "ACADEMY_CLOUD_FILE_NOT_FOUND"},
		429: {503, "ACADEMY_CLOUD_RATE_LIMITED"},
		502: {503, "ACADEMY_CLOUD_PROVIDER_UNAVAILABLE"},
	}
	for upstream, want := range cases {
		w := httptest.NewRecorder()
		academyCloudHTTPError(w, httptest.NewRequest("GET", "/", nil), academy.YandexStatusError{Status: upstream}, academy.ErrUnavailable)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("non-JSON error %q", w.Body.String())
		}
		if w.Code != want.status || body["code"] != want.code {
			t.Fatalf("%d got %d %v", upstream, w.Code, body["code"])
		}
	}
}
