package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
)

func compatibilityPolicy(t *testing.T, platform clientcompat.Platform) clientcompat.Policy {
	t.Helper()
	minimum, err := clientcompat.ParseVersion("1.4.0")
	if err != nil {
		t.Fatal(err)
	}
	recommended, err := clientcompat.ParseVersion("1.6.0")
	if err != nil {
		t.Fatal(err)
	}
	return clientcompat.Policy{
		Platform:                  platform,
		MinimumSupported:          minimum,
		Recommended:               recommended,
		ContractVersion:           "contract-v2",
		SupportedContractVersions: []string{"contract-v1", "contract-v2"},
		PolicyVersion:             "mobile-compat-v7",
		MinimumUpdateReason:       clientcompat.IncompatibleCritical,
		UpdateURL:                 "https://apgic.ru/update",
	}
}

func TestMobileCompatibilityKeepsSupportedPreviousContractWorking(t *testing.T) {
	handler := New(Options{
		ClientCompatibilityPolicies: map[clientcompat.Platform]clientcompat.Policy{
			clientcompat.IOS: compatibilityPolicy(t, clientcompat.IOS),
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/mobile/compatibility?platform=IOS&app_version=1.5.0&contract_version=contract-v1", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{
		`"status":"DEPRECATED_BUT_SUPPORTED"`,
		`"reason_code":"CLIENT_VERSION_DEPRECATED"`,
		`"policy_version":"mobile-compat-v7"`,
		`"contract_version":"contract-v2"`,
	} {
		if !contains(recorder.Body.String(), expected) {
			t.Fatalf("response missing %s: %s", expected, recorder.Body.String())
		}
	}
	if contains(recorder.Body.String(), "update_url") || contains(recorder.Body.String(), "update_reason") {
		t.Fatalf("supported client must not receive forced-update fields: %s", recorder.Body.String())
	}
}

func TestMobileCompatibilityUsesGovernedUpdateForUnsupportedContract(t *testing.T) {
	handler := New(Options{
		ClientCompatibilityPolicies: map[clientcompat.Platform]clientcompat.Policy{
			clientcompat.Android: compatibilityPolicy(t, clientcompat.Android),
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/mobile/compatibility?platform=ANDROID&app_version=1.6.0&contract_version=contract-v0", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{
		`"status":"UPDATE_REQUIRED"`,
		`"reason_code":"CLIENT_CONTRACT_UNSUPPORTED"`,
		`"update_reason":"INCOMPATIBLE_CRITICAL"`,
		`"update_url":"https://apgic.ru/update"`,
	} {
		if !contains(recorder.Body.String(), expected) {
			t.Fatalf("response missing %s: %s", expected, recorder.Body.String())
		}
	}
}

func TestMobileCompatibilityFailsClosedWithoutGovernedPolicy(t *testing.T) {
	handler := New(Options{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/mobile/compatibility?platform=IOS&app_version=1.6.0&contract_version=contract-v1", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !contains(recorder.Body.String(), "CLIENT_COMPATIBILITY_POLICY_UNAVAILABLE") {
		t.Fatalf("missing fail-closed reason: %s", recorder.Body.String())
	}
}
