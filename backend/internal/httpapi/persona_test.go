package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persona"
	"net/http/httptest"
	"testing"
	"time"
)

type personaStoreFake struct {
	deny    bool
	session persona.Session
	owner   string
	failed  bool
	catalog []persona.CatalogItem
}

func (s *personaStoreFake) ListPersonas(context.Context, string, string) ([]persona.CatalogItem, error) {
	return s.catalog, nil
}
func (s *personaStoreFake) BeginAvatar(_ context.Context, id, provider, _, policy string, request persona.Start) (persona.Session, error) {
	s.owner = id
	s.session = persona.Session{ID: "00000000-0000-0000-0000-000000000301", UserID: id, PersonaID: request.PersonaID, ProviderInstanceID: provider,
		Purpose: request.Purpose, PrivacyMode: request.PrivacyMode, Mode: request.Mode, Style: request.Style, Language: request.Language,
		VoiceProfile: "voice", VisualProfile: "visual", State: persona.StatePending, SafetyState: "NORMAL", PolicyVersion: policy}
	return s.session, nil
}
func (s *personaStoreFake) ActivateAvatar(_ context.Context, id, owner, _, mode string) error {
	if owner != s.owner {
		return persona.ErrNotFound
	}
	s.session.State = persona.StateActive
	s.session.Mode = mode
	s.session.ProviderSessionRef = "external"
	return nil
}
func (s *personaStoreFake) FailAvatar(context.Context, string, string) error {
	s.failed = true
	return nil
}
func (s *personaStoreFake) EndAvatar(context.Context, string, string) error {
	s.session.State = persona.StateEnded
	return nil
}
func (s *personaStoreFake) HandoffAvatar(context.Context, string, string) error {
	t := time.Now()
	s.session.HandoffRequestedAt = &t
	return nil
}
func (s *personaStoreFake) EscalateAvatar(context.Context, string, string) error {
	s.session.SafetyState = "ESCALATED"
	return nil
}
func (s *personaStoreFake) AuthorizeAvatarTurn(_ context.Context, _, owner string) error {
	if s.deny || owner != s.owner || s.session.State != persona.StateActive || s.session.HandoffRequestedAt != nil || s.session.SafetyState == "ESCALATED" {
		return persona.ErrConflict
	}
	return nil
}
func (s *personaStoreFake) ReadActiveAvatar(_ context.Context, owner, _, _ string) (persona.Session, error) {
	if owner != s.owner || s.session.State != persona.StateActive {
		return persona.Session{}, persona.ErrNotFound
	}
	return s.session, nil
}
func (s *personaStoreFake) ReadAvatar(_ context.Context, _, owner string) (persona.Session, error) {
	if owner != s.owner {
		return persona.Session{}, persona.ErrNotFound
	}
	return s.session, nil
}

type personaProviderFake struct {
	fail       bool
	failTurn   bool
	gone       bool
	failStatus bool
}

func (p personaProviderFake) Open(context.Context, connector.PersonaOpen) (connector.PersonaTransport, error) {
	if p.fail {
		return connector.PersonaTransport{}, connector.ErrPersonaTransport
	}
	return connector.PersonaTransport{SessionRef: "external", LaunchURL: "https://avatar.example/run", Mode: "VOICE", ExpiresAt: time.Now().Add(3 * time.Minute)}, nil
}
func (personaProviderFake) Close(context.Context, string) error { return nil }
func (p personaProviderFake) Active(_ context.Context, reference string) (bool, error) {
	if p.failStatus {
		return false, connector.ErrPersonaTransport
	}
	return reference == "external" && !p.gone, nil
}

func (p personaProviderFake) Turn(_ context.Context, ref, input string) (string, error) {
	if p.failTurn {
		return "", connector.ErrPersonaTransport
	}
	if ref != "external" || input == "" {
		return "", connector.ErrPersonaTransport
	}
	return "Ответ настоящего провайдера: " + input, nil
}
func TestPersonaHTTPDisabledAndPrivacy(t *testing.T) {
	key := bytes.Repeat([]byte("x"), 32)
	handler := New(Options{ClientSessionKey: key, PersonaEnabled: false})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "/v1/personas", nil))
	if res.Code != 200 || !bytes.Contains(res.Body.Bytes(), []byte(`"available":false`)) {
		t.Fatal(res.Code, res.Body.String())
	}
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, httptest.NewRequest("POST", "/v1/avatar-sessions", bytes.NewBufferString("{}")))
	if bad.Code != 503 {
		t.Fatal(bad.Code)
	}
}
func TestPersonaHTTPStartHandoffAndOwnerBoundary(t *testing.T) {
	store := &personaStoreFake{catalog: []persona.CatalogItem{{ID: "00000000-0000-0000-0000-000000000201", Label: "Test", SupportsVoice: true}}}
	handler := New(Options{ClientSessionKey: bytes.Repeat([]byte("k"), 32), PersonaStore: store,
		PersonaProvider: personaProviderFake{}, PersonaEnabled: true, PersonaProviderInstanceID: "provider", PersonaPolicyVersion: "v1"})
	body := `{"persona_id":"00000000-0000-0000-0000-000000000201","session_purpose":"LISTEN","mode":"VOICE","language":"ru","style":"CALM","privacy_mode":"PRIVATE_EPHEMERAL"}`
	req := httptest.NewRequest("POST", "/v1/avatar-sessions", bytes.NewBufferString(body))
	req.Header.Set("Idempotency-Key", "journey-01")
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, req)
	if start.Code != 201 {
		t.Fatal(start.Code, start.Body.String())
	}
	if bytes.Contains(start.Body.Bytes(), []byte("external")) {
		t.Fatal("provider reference leaked")
	}
	if len(start.Result().Cookies()) == 0 {
		t.Fatal("identity cookie missing")
	}
	cookie := start.Result().Cookies()[0]
	handoff := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/avatar-sessions/00000000-0000-0000-0000-000000000301/actions", bytes.NewBufferString(`{"action":"HUMAN_HANDOFF"}`))
	r.AddCookie(cookie)
	handler.ServeHTTP(handoff, r)
	if handoff.Code != 200 || !bytes.Contains(handoff.Body.Bytes(), []byte("NEW_HELP_INTENT_REQUIRED")) {
		t.Fatal(handoff.Code, handoff.Body.String())
	}
	if bytes.Contains(handoff.Body.Bytes(), []byte("external")) {
		t.Fatal("provider ref leaked on handoff")
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/v1/avatar-sessions/00000000-0000-0000-0000-000000000301", nil))
	if unauthorized.Code == 200 {
		t.Fatal("no owner session must not read")
	}
	// JSON carries only launch material and canonical lifecycle facts.
	var decoded map[string]any
	if err := json.Unmarshal(start.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
}
func TestPersonaProviderFailureDoesNotPretendConversation(t *testing.T) {
	store := &personaStoreFake{}
	handler := New(Options{ClientSessionKey: bytes.Repeat([]byte("s"), 32), PersonaStore: store,
		PersonaProvider: personaProviderFake{fail: true}, PersonaEnabled: true, PersonaProviderInstanceID: "provider", PersonaPolicyVersion: "v1"})
	req := httptest.NewRequest("POST", "/v1/avatar-sessions", bytes.NewBufferString(
		`{"persona_id":"00000000-0000-0000-0000-000000000201","session_purpose":"LISTEN","mode":"VOICE","language":"ru","style":"NEUTRAL","privacy_mode":"PRIVATE_EPHEMERAL"}`))
	req.Header.Set("Idempotency-Key", "failure-01")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 503 || !store.failed {
		t.Fatal(res.Code, res.Body.String())
	}
}

func TestPersonaActualDialogueAndProviderOutage(t *testing.T) {
	for _, failTurn := range []bool{false, true} {
		store := &personaStoreFake{catalog: []persona.CatalogItem{{ID: "00000000-0000-0000-0000-000000000201", SupportsText: true}}}
		handler := New(Options{ClientSessionKey: bytes.Repeat([]byte("k"), 32), PersonaStore: store,
			PersonaProvider: personaProviderFake{failTurn: failTurn}, PersonaEnabled: true, PersonaProviderInstanceID: "provider", PersonaPolicyVersion: "v1"})
		start := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/avatar-sessions", bytes.NewBufferString(`{"persona_id":"00000000-0000-0000-0000-000000000201","session_purpose":"LISTEN","mode":"TEXT","language":"ru","style":"CALM","privacy_mode":"PRIVATE_EPHEMERAL"}`))
		req.Header.Set("Idempotency-Key", "conversation-01")
		handler.ServeHTTP(start, req)
		if start.Code != 201 {
			t.Fatalf("start: %d %s", start.Code, start.Body)
		}
		sessionID := store.session.ID
		turn := httptest.NewRecorder()
		tr := httptest.NewRequest("POST", "/v1/avatar-sessions/"+sessionID+"/turns", bytes.NewBufferString(`{"message":"Привет"}`))
		tr.AddCookie(start.Result().Cookies()[0])
		handler.ServeHTTP(turn, tr)
		if failTurn {
			if turn.Code != 503 || bytes.Contains(turn.Body.Bytes(), []byte("Ответ настоящего")) {
				t.Fatalf("provider outage invented answer: %d %s", turn.Code, turn.Body)
			}
		} else if turn.Code != 200 || !bytes.Contains(turn.Body.Bytes(), []byte("MODEL_GENERATED")) || !bytes.Contains(turn.Body.Bytes(), []byte("Привет")) {
			t.Fatalf("real provider dialogue missing: %d %s", turn.Code, turn.Body)
		}
		if !failTurn {
			end := httptest.NewRecorder()
			er := httptest.NewRequest("POST", "/v1/avatar-sessions/"+sessionID+"/actions", bytes.NewBufferString(`{"action":"END"}`))
			er.AddCookie(start.Result().Cookies()[0])
			handler.ServeHTTP(end, er)
			if end.Code != 200 {
				t.Fatalf("end: %d %s", end.Code, end.Body)
			}
			denied := httptest.NewRecorder()
			dr := httptest.NewRequest("POST", "/v1/avatar-sessions/"+sessionID+"/turns", bytes.NewBufferString(`{"message":"Ещё"}`))
			dr.AddCookie(start.Result().Cookies()[0])
			handler.ServeHTTP(denied, dr)
			if denied.Code == 200 {
				t.Fatal("provider turn allowed after END")
			}
		}
	}
}

// The public catalog is the INTERSECTION of approved APGIC rights and a
// currently executable provider identity. No phantom selected characters.
type personaCatalogProviderFake struct {
	personaProviderFake
	catalog []connector.PersonaCapability
}

func (p personaCatalogProviderFake) Ready(context.Context) bool { return true }
func (p personaCatalogProviderFake) Catalog(context.Context) ([]connector.PersonaCapability, error) {
	return p.catalog, nil
}
func TestPersonaCatalogFiltersUnreviewedProviderIdentity(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000201"
	store := &personaStoreFake{catalog: []persona.CatalogItem{{
		ID: id, Label: "Ёж", Language: "ru", VoiceProfile: "licensed-voice",
		VisualProfile: "licensed-image", SupportsText: true, SupportsVoice: true,
	}}}
	provider := personaCatalogProviderFake{catalog: []connector.PersonaCapability{{
		ID: id, Language: "ru", VoiceProfile: "different-unreviewed-voice",
		VisualProfile: "licensed-image", SupportsText: true, SupportsVoice: true,
	}}}
	handler := New(Options{ClientSessionKey: bytes.Repeat([]byte("k"), 32), PersonaStore: store,
		PersonaProvider: provider, PersonaEnabled: true, PersonaProviderInstanceID: "provider", PersonaPolicyVersion: "v1"})
	call := func() (int, map[string]any) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/v1/personas", nil))
		var parsed map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
			t.Fatal(err)
		}
		return response.Code, parsed
	}
	status, result := call()
	if status != 200 || result["available"] != false {
		t.Fatalf("unreviewed voice shown as available: %+v", result)
	}
	provider.catalog[0].VoiceProfile = "licensed-voice"
	provider.catalog[0].SupportsVoice = false
	// Interface value captured a shallow slice; mutation updates the same data.
	status, result = call()
	if status != 200 || result["available"] != true {
		t.Fatalf("rights-cleared executable persona missing: %+v", result)
	}
	items, ok := result["personas"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("wrong catalog: %+v", result)
	}
	row, ok := items[0].(map[string]any)
	if !ok || row["supports_voice"] != false || row["supports_text"] != true {
		t.Fatalf("provider capability not enforced: %+v", row)
	}
}

// Real UI reload loses in-memory transcript but must recover session authorization
// from its HttpOnly owner cookie. A provider restart invalidates stale DB state.
func TestPersonaResumeOwnershipProviderRecoveryAndModelRevocation(t *testing.T) {
	for _, scenario := range []string{"ALIVE", "PROVIDER_LOST", "PROVIDER_OUTAGE", "POLICY_REVOKED"} {
		store := &personaStoreFake{catalog: []persona.CatalogItem{{
			ID: "00000000-0000-0000-0000-000000000201", SupportsVoice: true,
		}}}
		provider := personaProviderFake{gone: scenario == "PROVIDER_LOST", failStatus: scenario == "PROVIDER_OUTAGE"}
		handler := New(Options{ClientSessionKey: bytes.Repeat([]byte("x"), 32), PersonaStore: store,
			PersonaProvider: provider, PersonaEnabled: true, PersonaProviderInstanceID: "provider", PersonaPolicyVersion: "v1"})
		start := httptest.NewRecorder()
		input := httptest.NewRequest("POST", "/v1/avatar-sessions", bytes.NewBufferString(
			`{"persona_id":"00000000-0000-0000-0000-000000000201","session_purpose":"LISTEN","mode":"VOICE","language":"ru","style":"CALM","privacy_mode":"PRIVATE_EPHEMERAL"}`))
		input.Header.Set("Idempotency-Key", "resume-journey-01")
		handler.ServeHTTP(start, input)
		if start.Code != 201 {
			t.Fatalf("%s: start %d %s", scenario, start.Code, start.Body)
		}
		cookies := start.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("owner cookie missing")
		}
		if scenario == "POLICY_REVOKED" {
			store.deny = true
		}
		blocked := httptest.NewRecorder()
		handler.ServeHTTP(blocked, httptest.NewRequest("GET", "/v1/avatar-sessions/current", nil))
		if blocked.Code == 200 && !bytes.Contains(blocked.Body.Bytes(), []byte(`"session":null`)) {
			t.Fatalf("%s: another user recovered private session: %s", scenario, blocked.Body)
		}
		restored := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/avatar-sessions/current", nil)
		req.AddCookie(cookies[0])
		handler.ServeHTTP(restored, req)
		switch scenario {
		case "ALIVE":
			if restored.Code != 200 || !bytes.Contains(restored.Body.Bytes(), []byte(store.session.ID)) ||
				bytes.Contains(restored.Body.Bytes(), []byte("external")) {
				t.Fatalf("valid reload must restore public session only: %d %s", restored.Code, restored.Body)
			}
		case "PROVIDER_LOST", "POLICY_REVOKED":
			if restored.Code != 200 || store.session.State != persona.StateEnded ||
				!bytes.Contains(restored.Body.Bytes(), []byte(`"session":null`)) {
				t.Fatalf("stale provider/policy survived: %s %d %s", scenario, restored.Code, restored.Body)
			}
		case "PROVIDER_OUTAGE":
			if restored.Code != 503 || store.session.State != persona.StateActive {
				t.Fatalf("unknown provider outage treated as definite close: %d %s", restored.Code, restored.Body)
			}
		}
	}
}
