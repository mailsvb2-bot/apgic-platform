package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func TestMobileWorkspaceHTTPSwitchesOneIdentityAcrossRolesAndDeniesForeignWorkspace(t *testing.T) {
	databaseURL := os.Getenv("APGIC_IDENTITY_HTTP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("identity HTTP integration database not configured")
	}

	store, err := runtimeStoreForWorkspaceTest(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("w", 32))
	handler := New(Options{
		Demand:           service,
		Specialists:      store,
		Organizations:    store,
		MobileWorkspaces: store,
		ClientSessionKey: key,
	})

	identityID, session := createWorkspaceTestIdentity(t, handler, "workspace primary identity")
	upsertWorkspaceTestSpecialist(t, handler, session)
	organizationID := createWorkspaceTestOrganization(t, handler, session, "Workspace Primary Org")

	list := httptest.NewRequest(http.MethodGet, "/v1/mobile/workspaces", nil)
	list.AddCookie(session)
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("workspace list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed mobileWorkspaceListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Workspaces) != 3 {
		t.Fatalf("workspaces=%#v want exactly CLIENT+SPECIALIST+ORGANIZATION", listed.Workspaces)
	}
	kinds := map[string]authorizedWorkspaceResponse{}
	for _, workspace := range listed.Workspaces {
		if workspace.IdentityID != identityID || workspace.AuthorizationDecision != "ALLOW" {
			t.Fatalf("workspace escaped canonical identity/authorization: %#v", workspace)
		}
		kinds[string(workspace.Kind)] = workspace
	}
	for _, kind := range []string{"CLIENT", "SPECIALIST", "ORGANIZATION"} {
		if _, ok := kinds[kind]; !ok {
			t.Fatalf("missing %s workspace in %#v", kind, listed.Workspaces)
		}
	}
	if kinds["CLIENT"].TenantID != identityID || kinds["SPECIALIST"].TenantID != identityID {
		t.Fatalf("identity workspaces must retain identity tenant: %#v", listed.Workspaces)
	}
	if kinds["ORGANIZATION"].TenantID != organizationID {
		t.Fatalf("organization workspace tenant=%s want=%s", kinds["ORGANIZATION"].TenantID, organizationID)
	}

	ownResolution := httptest.NewRequest(
		http.MethodGet,
		"/v1/mobile/workspaces/"+kinds["ORGANIZATION"].WorkspaceID,
		nil,
	)
	ownResolution.AddCookie(session)
	ownRecorder := httptest.NewRecorder()
	handler.ServeHTTP(ownRecorder, ownResolution)
	if ownRecorder.Code != http.StatusOK {
		t.Fatalf("own workspace resolution status=%d body=%s", ownRecorder.Code, ownRecorder.Body.String())
	}
	var own mobileWorkspaceResolutionResponse
	if err := json.Unmarshal(ownRecorder.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if !own.Allowed || own.Workspace == nil || own.Workspace.TenantID != organizationID {
		t.Fatalf("own workspace did not resolve canonically: %#v", own)
	}

	_, foreignSession := createWorkspaceTestIdentity(t, handler, "workspace foreign identity")
	foreignOrganizationID := createWorkspaceTestOrganization(t, handler, foreignSession, "Workspace Foreign Org")
	forgedWorkspaceID := "organization:" + foreignOrganizationID

	forged := httptest.NewRequest(http.MethodGet, "/v1/mobile/workspaces/"+forgedWorkspaceID, nil)
	forged.AddCookie(session)
	forgedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(forgedRecorder, forged)
	if forgedRecorder.Code != http.StatusOK {
		t.Fatalf("forged workspace status=%d body=%s", forgedRecorder.Code, forgedRecorder.Body.String())
	}
	var denied mobileWorkspaceResolutionResponse
	if err := json.Unmarshal(forgedRecorder.Body.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Workspace != nil || denied.ReasonCode != "WORKSPACE_AUTHORIZATION_DENY" {
		t.Fatalf("foreign workspace leaked or was authorized: %#v", denied)
	}
	if strings.Contains(forgedRecorder.Body.String(), foreignOrganizationID) {
		t.Fatalf("foreign tenant identifier leaked in denial body: %s", forgedRecorder.Body.String())
	}
}

func runtimeStoreForWorkspaceTest(databaseURL string) (*runtimepostgres.Checker, error) {
	return runtimepostgres.Open(context.Background(), databaseURL)
}

func createWorkspaceTestIdentity(t *testing.T, handler http.Handler, freeText string) (string, *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/help-intents",
		strings.NewReader(`{"free_text":"`+freeText+`"}`),
	)
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create identity status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var intent demand.Intent
	if err := json.Unmarshal(recorder.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	var session *http.Cookie
	for _, candidate := range recorder.Result().Cookies() {
		if candidate.Name == clientSessionCookieName {
			session = candidate
			break
		}
	}
	if session == nil || intent.ClientIdentityID == "" {
		t.Fatalf("identity/session missing: identity=%q cookies=%v", intent.ClientIdentityID, recorder.Result().Cookies())
	}
	return intent.ClientIdentityID, session
}

func upsertWorkspaceTestSpecialist(t *testing.T, handler http.Handler, session *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/specialist/profile",
		strings.NewReader(`{"display_name":"Workspace Specialist","profession_code":"PSYCHOLOGIST"}`),
	)
	request.Header.Set("content-type", "application/json")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upsert specialist status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func createWorkspaceTestOrganization(t *testing.T, handler http.Handler, session *http.Cookie, name string) string {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations",
		strings.NewReader(`{"name":"`+name+`"}`),
	)
	request.Header.Set("content-type", "application/json")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create organization status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID == "" {
		t.Fatalf("organization id missing: %s", recorder.Body.String())
	}
	return payload.ID
}
