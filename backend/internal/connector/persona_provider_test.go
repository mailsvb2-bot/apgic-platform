package connector

import (
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
