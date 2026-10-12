package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PersonaTransport is a short-lived, provider-owned transport capability.
// This is NOT APGIC booking/identity/memory/entitlement authority.
type PersonaTransport struct {
	SessionRef string    `json:"session_ref"`
	LaunchURL  string    `json:"launch_url"`
	Mode       string    `json:"mode"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type PersonaOpen struct {
	APGICSessionID string `json:"apgic_session_id"`
	PersonaID      string `json:"persona_id"`
	Purpose        string `json:"session_purpose"`
	PrivacyMode    string `json:"privacy_mode"`
	Mode           string `json:"mode"`
	Style          string `json:"style"`
	Language       string `json:"language"`
	VoiceProfile   string `json:"voice_profile"`
	VisualProfile  string `json:"visual_profile"`
	PolicyVersion  string `json:"policy_version"`
}
type PersonaRuntime interface {
	Open(context.Context, PersonaOpen) (PersonaTransport, error)
	Close(context.Context, string) error
	Turn(context.Context, string, string) (string, error)
}

var ErrPersonaTransport = errors.New("PERSONA_TRANSPORT_UNAVAILABLE")

type HTTPPersonaRuntime struct {
	endpoint       string
	token          string
	transportHosts map[string]bool
	client         *http.Client
}

func NewHTTPPersonaRuntime(endpoint, token, allowedHosts string) (*HTTPPersonaRuntime, error) {
	endpoint = strings.TrimSpace(endpoint)
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Path != "" {
		return nil, fmt.Errorf("persona provider endpoint must be an HTTPS origin without path/query")
	}
	if len(strings.TrimSpace(token)) < 24 {
		return nil, fmt.Errorf("persona provider service credential required")
	}
	hosts := map[string]bool{}
	for _, h := range strings.Split(allowedHosts, ",") {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" || strings.ContainsAny(h, ":/?#@ ") {
			continue
		}
		hosts[h] = true
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("explicit persona transport hostname allowlist required")
	}
	return &HTTPPersonaRuntime{endpoint: strings.TrimRight(endpoint, "/"), token: token, transportHosts: hosts,
		client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (p *HTTPPersonaRuntime) call(ctx context.Context, path string, payload any, result any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return ErrPersonaTransport
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ErrPersonaTransport
	}
	if result == nil {
		return nil
	}
	res.Body = io.NopCloser(io.LimitReader(res.Body, 8193))
	decoder := json.NewDecoder(res.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return ErrPersonaTransport
	}
	if n, err := io.Copy(io.Discard, res.Body); err != nil || n > 0 {
		return ErrPersonaTransport
	}
	return nil
}
func (p *HTTPPersonaRuntime) Open(ctx context.Context, r PersonaOpen) (PersonaTransport, error) {
	if p == nil {
		return PersonaTransport{}, ErrPersonaTransport
	}
	var out PersonaTransport
	if err := p.call(ctx, "/v1/persona-sessions", r, &out); err != nil {
		return PersonaTransport{}, err
	}
	u, err := url.Parse(out.LaunchURL)
	if (out.LaunchURL != "" && (err != nil || u.Scheme != "https" || !p.transportHosts[strings.ToLower(u.Hostname())] || u.User != nil || u.Fragment != "")) ||
		(out.LaunchURL == "" && out.Mode == "VIDEO") || strings.TrimSpace(out.SessionRef) == "" ||
		time.Until(out.ExpiresAt) <= 0 || time.Until(out.ExpiresAt) > 15*time.Minute || !validPersonaMode(out.Mode, r.Mode) {
		return PersonaTransport{}, ErrPersonaTransport
	}
	return out, nil
}
func validPersonaMode(actual, requested string) bool {
	switch requested {
	case "VIDEO":
		return actual == "VIDEO" || actual == "VOICE" || actual == "TEXT"
	case "VOICE":
		return actual == "VOICE" || actual == "TEXT"
	case "TEXT":
		return actual == "TEXT"
	}
	return false
}
func (p *HTTPPersonaRuntime) Close(ctx context.Context, reference string) error {
	if p == nil || strings.TrimSpace(reference) == "" {
		return ErrPersonaTransport
	}
	// Reference is a body value: never interpolate provider-controlled IDs into URLs.
	return p.call(ctx, "/v1/persona-sessions/close", map[string]string{"session_ref": reference}, nil)
}

// Turn executes model-backed dialogue on the external ephemeral runtime. APGIC never
// persists inputs or outputs. The provider must fail, not invent a reply, on outage.
func (p *HTTPPersonaRuntime) Turn(ctx context.Context, ref, message string) (string, error) {
	message = strings.TrimSpace(message)
	if p == nil || strings.TrimSpace(ref) == "" || message == "" || len(message) > 2000 {
		return "", ErrPersonaTransport
	}
	var out struct {
		Reply  string `json:"reply"`
		Source string `json:"source"`
	}
	if err := p.call(ctx, "/v1/persona-sessions/turns", map[string]string{"session_ref": ref, "message": message}, &out); err != nil {
		return "", ErrPersonaTransport
	}
	out.Reply = strings.TrimSpace(out.Reply)
	if out.Source != "MODEL_GENERATED" || out.Reply == "" || len(out.Reply) > 4500 {
		return "", ErrPersonaTransport
	}
	return out.Reply, nil
}

// Ready probes actual model-backed provider process health; a configured
// catalog without a reachable provider must not create a false CTA.
func (p *HTTPPersonaRuntime) Ready(ctx context.Context) bool {
	if p == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/healthz", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	res, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false
	}
	var state struct {
		Status string `json:"status"`
		Mode   string `json:"mode"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 512)).Decode(&state) != nil {
		return false
	}
	return state.Status == "ok" && state.Mode == "MODEL_BACKED"
}

// PersonaCapability is provider readiness metadata, never APGIC business truth.
// A matching ID does not grant public rights without PostgreSQL approval.
type PersonaCapability struct {
	ID            string `json:"id"`
	Language      string `json:"language"`
	VoiceProfile  string `json:"voice_profile"`
	VisualProfile string `json:"visual_profile"`
	SupportsText  bool   `json:"supports_text"`
	SupportsVoice bool   `json:"supports_voice"`
}

func (p *HTTPPersonaRuntime) Catalog(ctx context.Context) ([]PersonaCapability, error) {
	if p == nil {
		return nil, ErrPersonaTransport
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/v1/persona-catalog", nil)
	if err != nil {
		return nil, ErrPersonaTransport
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, ErrPersonaTransport
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrPersonaTransport
	}
	var out struct {
		Personas []PersonaCapability `json:"personas"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || len(out.Personas) > 100 {
		return nil, ErrPersonaTransport
	}
	return out.Personas, nil
}

// Active verifies a specific ephemeral lease with the provider after a client
// reload or API restart. No transcript, provider ref or token reaches clients.
func (p *HTTPPersonaRuntime) Active(ctx context.Context, reference string) (bool, error) {
	if p == nil || strings.TrimSpace(reference) == "" {
		return false, ErrPersonaTransport
	}
	var status struct {
		Active bool `json:"active"`
	}
	if err := p.call(ctx, "/v1/persona-sessions/status",
		map[string]string{"session_ref": reference}, &status); err != nil {
		return false, ErrPersonaTransport
	}
	return status.Active, nil
}
