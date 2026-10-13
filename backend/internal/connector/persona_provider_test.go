package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPersonaConnectorFailsClosed(t *testing.T) {
	for _, v := range []struct{ url, token, hosts string }{
		{"http://test.invalid", "credential-with-sufficient-length", "test.invalid"},
		{"https://test.invalid/path", "credential-with-sufficient-length", "test.invalid"},
		{"https://test.invalid", "short", "test.invalid"},
		{"https://test.invalid", "credential-with-sufficient-length", ""},
	} {
		if _, err := NewHTTPPersonaRuntime(v.url, v.token, v.hosts); err == nil {
			t.Fatalf("accepted unsafe connector %+v", v)
		}
	}
	if _, err := NewHTTPPersonaRuntime("https://test.invalid", "credential-with-sufficient-length", "persona.example"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		got, asked string
		valid      bool
	}{
		{"VOICE", "VIDEO", true}, {"TEXT", "VIDEO", true}, {"VIDEO", "VOICE", false}, {"VOICE", "TEXT", false},
	} {
		if validPersonaMode(v.got, v.asked) != v.valid {
			t.Fatalf("mode: %+v", v)
		}
	}
	_ = time.Now()
}

func TestPersonaConnectorRequiresRealModelReply(t *testing.T) {
	var turns int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("k", 32) {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "mode": "MODEL_BACKED"})
		case "/v1/persona-catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{"personas": []map[string]any{{
				"id": "00000000-0000-0000-0000-000000000201", "language": "ru",
				"voice_profile": "cleared-voice", "visual_profile": "cleared-image",
				"supports_text": true, "supports_voice": false,
			}}})
		case "/v1/persona-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"session_ref": "real-ephemeral-ref", "launch_url": "", "mode": "TEXT", "expires_at": time.Now().Add(5 * time.Minute)})
		case "/v1/persona-sessions/status":
			_ = json.NewEncoder(w).Encode(map[string]bool{"active": true})
		case "/v1/persona-sessions/turns":
			turns++
			_ = json.NewEncoder(w).Encode(map[string]string{"reply": "Настоящий ответ модели", "source": "MODEL_GENERATED"})
		case "/v1/persona-sessions/close":
			_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p, err := NewHTTPPersonaRuntime(server.URL, strings.Repeat("k", 32), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	p.client = server.Client()
	if !p.Ready(context.Background()) {
		t.Fatal("healthy external model-backed provider not detected")
	}
	list, listingError := p.Catalog(context.Background())
	if listingError != nil || len(list) != 1 || list[0].ID != "00000000-0000-0000-0000-000000000201" || list[0].SupportsVoice {
		t.Fatalf("provider capability contract invalid: %+v %v", list, listingError)
	}
	out, err := p.Open(context.Background(), PersonaOpen{Mode: "TEXT"})
	if err != nil || out.Mode != "TEXT" || out.LaunchURL != "" {
		t.Fatalf("inline conversation denied: %+v %v", out, err)
	}
	active, statusError := p.Active(context.Background(), out.SessionRef)
	if statusError != nil || !active {
		t.Fatalf("provider session status did not reconcile: %v %v", active, statusError)
	}
	reply, err := p.Turn(context.Background(), out.SessionRef, "Привет")
	if err != nil || reply != "Настоящий ответ модели" || turns != 1 {
		t.Fatalf("not a real turn: %q %v", reply, err)
	}
	if err := p.Close(context.Background(), out.SessionRef); err != nil {
		t.Fatal(err)
	}
}
func TestPersonaConnectorRejectsFakeReplies(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"reply": "Заглушка", "source": "MOCK"})
	}))
	defer server.Close()
	p, err := NewHTTPPersonaRuntime(server.URL, strings.Repeat("k", 32), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	p.client = server.Client()
	if p.Ready(context.Background()) {
		t.Fatal("unverified provider health accepted")
	}
	if text, err := p.Turn(context.Background(), "ref", "Привет"); err == nil || text != "" {
		t.Fatal("synthetic provider reply accepted")
	}
}
