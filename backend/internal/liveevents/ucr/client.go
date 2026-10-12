package ucr

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// Client calls the existing UCR Universal Conference API via a machine
// principal. Credentials and media grant signatures stay owned by UCR.
type Client struct {
	endpoint      *url.URL
	joinOrigin    string
	integrationID string
	providerID    string
	bearer        string
	httpClient    *http.Client
	now           func() time.Time
}

func NewFromEnvironment() (*Client, error) {
	endpoint := strings.TrimSpace(os.Getenv("APGIC_UCR_CONFERENCE_URL"))
	bearer := strings.TrimSpace(os.Getenv("APGIC_UCR_MACHINE_BEARER"))
	integration := strings.TrimSpace(os.Getenv("APGIC_UCR_INTEGRATION_ID"))
	provider := strings.TrimSpace(os.Getenv("APGIC_UCR_CONNECTOR_INSTANCE_ID"))
	joinOrigin := strings.TrimSpace(os.Getenv("APGIC_UCR_JOIN_ORIGIN"))
	if endpoint == "" && bearer == "" && integration == "" && provider == "" && joinOrigin == "" {
		return nil, nil
	}
	return NewClient(endpoint, bearer, integration, provider, joinOrigin, &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, time.Now)
}
func NewClient(endpoint, bearer, integration, provider, joinOrigin string, client *http.Client, now func() time.Time) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return nil, liveevents.ErrInvalid
	}
	j, err := url.Parse(joinOrigin)
	if err != nil || j.Host == "" || j.Scheme != "https" || j.User != nil || j.Path != "" || j.RawQuery != "" || j.Fragment != "" ||
		strings.TrimSpace(bearer) == "" || len(bearer) > 8192 || strings.ContainsAny(bearer, " \r\t\n") ||
		strings.TrimSpace(integration) == "" || strings.TrimSpace(provider) == "" || client == nil || now == nil {
		return nil, liveevents.ErrInvalid
	}
	return &Client{endpoint: u, joinOrigin: j.Scheme + "://" + j.Host, integrationID: integration, providerID: provider, bearer: bearer, httpClient: client, now: now}, nil
}
func (c *Client) request(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var reader io.Reader
	if payload != nil {
		bytes, err := json.Marshal(payload)
		if err != nil {
			return nil, liveevents.ErrInvalid
		}
		reader = bytespkg(bytes)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := *c.endpoint
	endpoint.Path = path
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, liveevents.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, liveevents.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, liveevents.ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return nil, liveevents.ErrUnavailable
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil || out == nil {
		return nil, liveevents.ErrUnavailable
	}
	return out, nil
}
func bytespkg(b []byte) io.Reader { return bytes.NewReader(b) }
func child(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}
func str(m map[string]any, key string) string        { v, _ := m[key].(string); return v }
func yes(m map[string]any, key string) bool          { v, _ := m[key].(bool); return v }
func num(m map[string]any, key string) float64       { v, _ := m[key].(float64); return v }
func (c *Client) scope(org string) map[string]string { return map[string]string{"tenant_id": org} }
func (c *Client) context(room liveevents.MediaRoom) map[string]any {
	return map[string]any{"scope": c.scope(room.OrganizationID), "integration_id": room.IntegrationID, "conference_id": room.ConferenceID}
}
func (c *Client) capabilities(ctx context.Context, org string, capacity int, format liveevents.Format) error {
	target := *c.endpoint
	target.Path = "/v1/capabilities"
	target.RawQuery = url.Values{"tenant_id": {org}, "integration_id": {c.integrationID}}.Encode()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		return liveevents.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return liveevents.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return liveevents.ErrUnavailable
	}
	var result map[string]any
	if json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&result) != nil {
		return liveevents.ErrUnavailable
	}
	caps := child(result, "capabilities")
	if !yes(caps, "browser_realtime_gateway") || !yes(caps, "production_webrtc") || !yes(caps, "turn") || !yes(caps, "video") || int(num(caps, "max_participants")) < capacity ||
		((format == liveevents.Webinar || format == liveevents.Conference) && !yes(caps, "webinar")) || (capacity > 100 && !yes(caps, "horizontal_sfu")) {
		return liveevents.ErrUnavailable
	}
	return nil
}
func (c *Client) Prepare(ctx context.Context, e liveevents.Event) (liveevents.MediaRoom, error) {
	if err := c.capabilities(ctx, e.OrganizationID, e.Capacity, e.Format); err != nil {
		return liveevents.MediaRoom{}, err
	}
	mode := "meeting"
	if e.Format == liveevents.Webinar || e.Format == liveevents.Conference {
		mode = "webinar"
	}
	result, err := c.request(ctx, "POST", "/v1/conferences", map[string]any{
		"scope": c.scope(e.OrganizationID), "integration_id": c.integrationID,
		"external_conference_id_b64": base64.StdEncoding.EncodeToString([]byte(e.ID)),
		"idempotency_key":            "apgic-r9-conference-" + e.ID, "mode": mode,
		"schedule": map[string]any{"starts_at_unix_ms": e.StartsAt.UnixMilli(), "planned_end_unix_ms": e.EndsAt.UnixMilli(), "join_before_seconds": 900, "join_after_seconds": 900},
	})
	if err != nil {
		return liveevents.MediaRoom{}, err
	}
	room := child(result, "conference")
	id := str(room, "conference_id")
	if id == "" || (str(room, "lifecycle") != "scheduled" && str(room, "lifecycle") != "waiting" && !(e.State == liveevents.Live && str(room, "lifecycle") == "live")) {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	// UCR runtime (not APGIC) must prove Group and Call preparation before
	// APGIC binds and publishes the room. Conference existence alone is not
	// evidence of usable media or admission.
	runtimeRequest := map[string]any{
		"scope":           c.scope(e.OrganizationID),
		"integration_id":  c.integrationID,
		"conference_id":   id,
		"idempotency_key": "apgic-r9-runtime-" + e.ID,
	}
	prepared, err := c.request(ctx, "POST", "/v1/conferences/runtime", runtimeRequest)
	if err != nil {
		return liveevents.MediaRoom{}, err
	}
	runtime := child(prepared, "runtime")
	if !yes(runtime, "group_ready") || !yes(runtime, "call_ready") {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	return liveevents.MediaRoom{EventID: e.ID, OrganizationID: e.OrganizationID, ProviderInstanceID: c.providerID,
		ConferenceID: id, IntegrationID: c.integrationID, ProviderReadyAt: c.now().UTC(),
		StartsAt: e.StartsAt, EndsAt: e.EndsAt, Capacity: e.Capacity, Format: e.Format, State: e.State}, nil
}
func (c *Client) Change(ctx context.Context, room liveevents.MediaRoom, target liveevents.State) error {
	if err := c.capabilities(ctx, room.OrganizationID, room.Capacity, room.Format); err != nil {
		return err
	}
	steps := []string{}
	switch target {
	case liveevents.Published:
		steps = []string{"waiting"}
	case liveevents.Live:
		steps = []string{"live"}
	case liveevents.Completed, liveevents.Cancelled:
		steps = []string{"ending", "ended"}
	default:
		return liveevents.ErrInvalid
	}
	for _, step := range steps {
		payload := c.context(room)
		payload["target"] = step
		payload["idempotency_key"] = "apgic-r9-state-" + room.EventID + "-" + step
		result, err := c.request(ctx, "POST", "/v1/conferences/lifecycle", payload)
		if err != nil {
			return err
		}
		if str(child(result, "conference"), "lifecycle") != step {
			return liveevents.ErrUnavailable
		}
	}
	if target == liveevents.Published {
		payload := c.context(room)
		payload["entry_open"] = true
		payload["idempotency_key"] = "apgic-r9-entry-" + room.EventID
		result, err := c.request(ctx, "POST", "/v1/conferences/entry", payload)
		if err != nil {
			return err
		}
		if !yes(child(result, "conference"), "entry_open") {
			return liveevents.ErrUnavailable
		}
	}
	return nil
}
func (c *Client) Join(ctx context.Context, room liveevents.MediaRoom, identity, role string) (liveevents.MediaGrant, error) {
	if role != "host" && role != "attendee" {
		return liveevents.MediaGrant{}, liveevents.ErrForbidden
	}
	if err := c.capabilities(ctx, room.OrganizationID, room.Capacity, room.Format); err != nil {
		return liveevents.MediaGrant{}, err
	}
	participant := c.context(room)
	encoded := base64.StdEncoding.EncodeToString([]byte(identity))
	participant["external_user_id_b64"] = encoded
	participant["role"] = role
	participant["idempotency_key"] = "apgic-r9-participant-" + room.EventID + "-" + identity
	result, err := c.request(ctx, "POST", "/v1/participants", participant)
	if err != nil {
		return liveevents.MediaGrant{}, err
	}
	if !yes(child(result, "participant"), "active") {
		return liveevents.MediaGrant{}, liveevents.ErrUnavailable
	}
	device := c.context(room)
	device["external_user_id_b64"] = encoded
	device["idempotency_key"] = "apgic-r9-device-" + room.EventID + "-" + identity
	result, err = c.request(ctx, "POST", "/v1/participant-devices", device)
	if err != nil {
		return liveevents.MediaGrant{}, err
	}
	if !yes(child(result, "device"), "active") {
		return liveevents.MediaGrant{}, liveevents.ErrUnavailable
	}
	request := c.context(room)
	request["external_user_id_b64"] = encoded
	request["ttl_seconds"] = 120
	request["use_policy"] = "single_use"
	// UCR single-use join grants need a fresh idempotency identity for each
	// new attempt; bucket reuse can strand reconnecting participants.
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return liveevents.MediaGrant{}, liveevents.ErrUnavailable
	}
	request["idempotency_key"] = "apgic-r9-join-" + room.EventID + "-" + identity + "-" + hex.EncodeToString(nonce)
	result, err = c.request(ctx, "POST", "/v1/join-grants", request)
	if err != nil {
		return liveevents.MediaGrant{}, err
	}
	grant := child(result, "grant")
	u, err := url.Parse(str(grant, "join_url"))
	expiry := int64(num(grant, "expires_at_unix_ms"))
	if err != nil || u.User != nil || u.Scheme != "https" || u.Scheme+"://"+u.Host != c.joinOrigin ||
		expiry <= c.now().UnixMilli() || expiry > c.now().Add(5*time.Minute).UnixMilli() {
		return liveevents.MediaGrant{}, liveevents.ErrUnavailable
	}
	frag, err := url.ParseQuery(u.Fragment)
	if err != nil || frag.Get("ucr_join") == "" {
		return liveevents.MediaGrant{}, liveevents.ErrUnavailable
	}
	return liveevents.MediaGrant{JoinURL: u.String(), ExpiresAt: time.UnixMilli(expiry).UTC()}, nil
}
func (c *Client) Moderate(ctx context.Context, room liveevents.MediaRoom, identity, action string) error {
	if action != "mute" && action != "remove" {
		return liveevents.ErrInvalid
	}
	request := c.context(room)
	request["external_user_id_b64"] = base64.StdEncoding.EncodeToString([]byte(identity))
	request["idempotency_key"] = "apgic-r9-moderate-" + room.EventID + "-" + identity + "-" + action
	path := "/v1/participants/remove"
	if action == "mute" {
		path = "/v1/participants/update"
		request["audio_muted"] = true
	}
	result, err := c.request(ctx, "POST", path, request)
	if err != nil {
		return err
	}
	if action == "mute" && !yes(child(result, "participant"), "audio_muted") {
		return liveevents.ErrUnavailable
	}
	return nil
}
func (c *Client) Attendance(ctx context.Context, room liveevents.MediaRoom, identity string) (liveevents.MediaParticipantFact, error) {
	payload := c.context(room)
	payload["external_user_id_b64"] = base64.StdEncoding.EncodeToString([]byte(identity))
	result, err := c.request(ctx, "POST", "/v1/attendance", payload)
	if err != nil {
		return liveevents.MediaParticipantFact{}, err
	}
	response := child(result, "attendance")
	count := int(num(response, "join_count"))
	duration := int64(num(response, "total_connected_seconds"))
	first := time.UnixMilli(int64(num(response, "first_join_at_unix_ms"))).UTC()
	if count < 0 || duration < 0 || (count > 0 && num(response, "first_join_at_unix_ms") <= 0) {
		return liveevents.MediaParticipantFact{}, fmt.Errorf("UCR attendance evidence invalid: %w", liveevents.ErrUnavailable)
	}
	return liveevents.MediaParticipantFact{Joined: count > 0, JoinCount: count, ConnectedSeconds: duration, FirstJoinedAt: first}, nil
}
