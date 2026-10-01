package runtimepostgres

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestMobileInstallationHTTPPreservesIdentityAcrossRotationReinstallAndRevoke(t *testing.T) {
	databaseURL := os.Getenv("APGIC_MOBILE_INSTALLATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("mobile installation integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := []byte(strings.Repeat("m", 32))
	handler := httpapi.New(httpapi.Options{
		Demand:           demand.NewConformanceService(nil),
		Installations:    store,
		ClientSessionKey: key,
	})

	bootstrap := func(freeText string) (*http.Cookie, string) {
		request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", strings.NewReader(`{"free_text":"`+freeText+`"}`))
		request.Header.Set("content-type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("bootstrap status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var intent demand.Intent
		if err := json.Unmarshal(recorder.Body.Bytes(), &intent); err != nil {
			t.Fatal(err)
		}
		var session *http.Cookie
		for _, candidate := range recorder.Result().Cookies() {
			if candidate.Name == "__Host-apgic_session" {
				session = candidate
				break
			}
		}
		if session == nil || intent.ClientIdentityID == "" {
			t.Fatalf("bootstrap missing trusted session or identity: cookie=%#v identity=%q", session, intent.ClientIdentityID)
		}
		return session, intent.ClientIdentityID
	}

	call := func(cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("content-type", "application/json")
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	decodeInstallation := func(recorder *httptest.ResponseRecorder) mobile.ClientInstallation {
		var value mobile.ClientInstallation
		if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	ownerCookie, ownerIdentity := bootstrap("installation owner")
	iosID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	androidID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	reinstallID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}

	registerIOS := call(ownerCookie, http.MethodPost, "/v1/mobile/installations",
		`{"id":"`+iosID+`","platform":"IOS","push_endpoint":"token-ios-a"}`)
	if registerIOS.Code != http.StatusCreated {
		t.Fatalf("register ios status=%d body=%s", registerIOS.Code, registerIOS.Body.String())
	}
	ios := decodeInstallation(registerIOS)
	if ios.IdentityID != ownerIdentity || ios.PushGeneration != 1 || !ios.CanReceivePush("token-ios-a", 1) {
		t.Fatalf("unexpected initial iOS installation: %#v", ios)
	}

	replay := call(ownerCookie, http.MethodPost, "/v1/mobile/installations",
		`{"id":"`+iosID+`","platform":"IOS","push_endpoint":"token-ios-a"}`)
	if replay.Code != http.StatusOK {
		t.Fatalf("registration replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if got := decodeInstallation(replay); got.PushGeneration != 1 || got.IdentityID != ownerIdentity {
		t.Fatalf("registration replay changed installation truth: %#v", got)
	}

	rotate := call(ownerCookie, http.MethodPatch, "/v1/mobile/installations/"+iosID+"/push-endpoint",
		`{"push_endpoint":"token-ios-b"}`)
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotate.Code, rotate.Body.String())
	}
	ios = decodeInstallation(rotate)
	if ios.PushGeneration != 2 || ios.CanReceivePush("token-ios-a", 1) || !ios.CanReceivePush("token-ios-b", 2) {
		t.Fatalf("push rotation did not invalidate stale generation: %#v", ios)
	}

	rotateReplay := call(ownerCookie, http.MethodPatch, "/v1/mobile/installations/"+iosID+"/push-endpoint",
		`{"push_endpoint":"token-ios-b"}`)
	if rotateReplay.Code != http.StatusOK || decodeInstallation(rotateReplay).PushGeneration != 2 {
		t.Fatalf("same-token rotation was not idempotent: status=%d body=%s", rotateReplay.Code, rotateReplay.Body.String())
	}

	registerAndroid := call(ownerCookie, http.MethodPost, "/v1/mobile/installations",
		`{"id":"`+androidID+`","platform":"ANDROID","push_endpoint":"token-android-a"}`)
	if registerAndroid.Code != http.StatusCreated {
		t.Fatalf("register android status=%d body=%s", registerAndroid.Code, registerAndroid.Body.String())
	}
	if got := decodeInstallation(registerAndroid); got.IdentityID != ownerIdentity || got.Platform != "ANDROID" {
		t.Fatalf("multi-device registration split identity: %#v", got)
	}

	reinstall := call(ownerCookie, http.MethodPost, "/v1/mobile/installations",
		`{"id":"`+reinstallID+`","platform":"IOS","push_endpoint":"token-ios-b"}`)
	if reinstall.Code != http.StatusCreated {
		t.Fatalf("reinstall status=%d body=%s", reinstall.Code, reinstall.Body.String())
	}
	if got := decodeInstallation(reinstall); got.IdentityID != ownerIdentity || got.ID != reinstallID {
		t.Fatalf("reinstall did not preserve identity: %#v", got)
	}

	list := call(ownerCookie, http.MethodGet, "/v1/mobile/installations", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Installations []mobile.ClientInstallation `json:"installations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Installations) != 3 {
		t.Fatalf("installation history count=%d want=3: %#v", len(listed.Installations), listed.Installations)
	}
	var oldIOS, currentIOS, android mobile.ClientInstallation
	for _, value := range listed.Installations {
		if value.IdentityID != ownerIdentity {
			t.Fatalf("installation escaped owner identity: %#v", value)
		}
		switch value.ID {
		case iosID:
			oldIOS = value
		case reinstallID:
			currentIOS = value
		case androidID:
			android = value
		}
	}
	if oldIOS.State != mobile.InstallationRevoked || oldIOS.PushEndpoint != "" {
		t.Fatalf("reinstall did not revoke stale installation: %#v", oldIOS)
	}
	if !currentIOS.CanReceivePush("token-ios-b", 1) || !android.CanReceivePush("token-android-a", 1) {
		t.Fatalf("current multi-device endpoints not active: ios=%#v android=%#v", currentIOS, android)
	}

	otherCookie, _ := bootstrap("other identity")
	crossRotate := call(otherCookie, http.MethodPatch, "/v1/mobile/installations/"+androidID+"/push-endpoint",
		`{"push_endpoint":"token-stolen"}`)
	if crossRotate.Code != http.StatusNotFound || !strings.Contains(crossRotate.Body.String(), "MOBILE_INSTALLATION_NOT_FOUND") {
		t.Fatalf("cross-identity installation mutation disclosed or succeeded: status=%d body=%s", crossRotate.Code, crossRotate.Body.String())
	}

	conflictID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	crossToken := call(otherCookie, http.MethodPost, "/v1/mobile/installations",
		`{"id":"`+conflictID+`","platform":"ANDROID","push_endpoint":"token-android-a"}`)
	if crossToken.Code != http.StatusConflict || !strings.Contains(crossToken.Body.String(), "MOBILE_PUSH_ENDPOINT_CONFLICT") {
		t.Fatalf("cross-identity push endpoint takeover was not blocked: status=%d body=%s", crossToken.Code, crossToken.Body.String())
	}

	revoke := call(ownerCookie, http.MethodPost, "/v1/mobile/installations/"+androidID+"/revoke", "")
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revoke.Code, revoke.Body.String())
	}
	revoked := decodeInstallation(revoke)
	if revoked.State != mobile.InstallationRevoked || revoked.PushEndpoint != "" || revoked.CanReceivePush("token-android-a", 1) {
		t.Fatalf("revoked endpoint remains eligible: %#v", revoked)
	}

	revokeReplay := call(ownerCookie, http.MethodPost, "/v1/mobile/installations/"+androidID+"/revoke", "")
	if revokeReplay.Code != http.StatusOK {
		t.Fatalf("revoke replay status=%d body=%s", revokeReplay.Code, revokeReplay.Body.String())
	}
	if got := decodeInstallation(revokeReplay); got.State != mobile.InstallationRevoked || got.IdentityID != ownerIdentity {
		t.Fatalf("revoke replay changed installation truth: %#v", got)
	}
}
