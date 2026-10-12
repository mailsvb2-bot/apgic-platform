package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func referenceFixture(t *testing.T) (*referenceService, time.Time) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Second)
	cfg := config{APGICIssuerKey: strings.Repeat("a", 32), APIKey: "test-api",
		APISecret: strings.Repeat("s", 48), MediaWebSocketURL: "wss://sfu.test.example",
		PublicOrigin: "https://media.example", ProviderID: "11111111-1111-4111-8111-111111111111",
		APGICWebhookURL: "https://apgic.example/v1/consultation-provider-webhooks",
		KeyID:           "ref-ed25519-v1", SigningKey: key, StateDir: t.TempDir(),
		MinimumSession: 60 * time.Second,
	}
	service := newReferenceService(cfg)
	service.clock = func() time.Time { return start }
	return service, start
}

func issueFixture(t *testing.T, s *referenceService, booking, id, role, key string, expiry time.Time) (string, issueResponse) {
	t.Helper()
	payload, err := json.Marshal(issueRequest{ContractVersion: "apgic.room-join.v1",
		ProviderInstanceID: s.config.ProviderID, BookingID: booking,
		IdentityID: id, Role: role, Scope: "ROOM_JOIN", IdempotencyKey: key, ExpiresAt: expiry})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/room-grants", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+s.config.APGICIssuerKey)
	req.Header.Set("X-APGIC-Contract", "room-join-v1")
	res := httptest.NewRecorder()
	s.handleIssue(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("room issuance rejected: %d %s", res.Code, res.Body.String())
	}
	var result issueResponse
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(result.JoinURI)
	if err != nil {
		t.Fatal(err)
	}
	grantToken := strings.TrimPrefix(target.Fragment, "token=")
	token, err := url.QueryUnescape(grantToken)
	if err != nil {
		t.Fatal(err)
	}
	return token, result
}

func providerEvent(id, kind, role, room string, when time.Time) livekitEvent {
	v := livekitEvent{ID: id, Event: kind, CreatedAt: when.Unix()}
	v.Room.Name = room
	v.Participant.Identity = role
	return v
}

func TestTwoParticipantsRealMediaEvidenceAndRestartSafeOutbox(t *testing.T) {
	s, start := referenceFixture(t)
	booking := "22222222-2222-4222-8222-222222222222"
	client := "33333333-3333-4333-8333-333333333333"
	specialist := "44444444-4444-4444-8444-444444444444"
	tokenClient, first := issueFixture(t, s, booking, client, "CLIENT", "client-idempotent-key-0001", start.Add(5*time.Minute))
	tokenSpecialist, second := issueFixture(t, s, booking, specialist, "SPECIALIST", "specialist-idempotent-001", start.Add(5*time.Minute))
	if first.JoinURI == second.JoinURI {
		t.Fatal("two roles received one bearer credential")
	}
	if first.ProviderCredentialRef == second.ProviderCredentialRef {
		t.Fatal("credential references collided")
	}
	_, replay := issueFixture(t, s, booking, client, "CLIENT", "client-idempotent-key-0001", start.Add(5*time.Minute))
	if first != replay {
		t.Fatalf("issuer did not replay deterministic credential: %+v vs %+v", first, replay)
	}
	identityClient := identityFor("CLIENT", client)
	identitySpecialist := identityFor("SPECIALIST", specialist)
	roomID := roomName(booking)

	apply := func(v livekitEvent) {
		t.Helper()
		s.mu.Lock()
		room, err := s.readRoom(booking)
		if err == nil {
			err = s.applyLiveKitEvent(room, v)
		}
		if err == nil {
			err = s.writeRoom(room)
		}
		s.mu.Unlock()
		if err != nil {
			t.Fatalf("provider evidence %s rejected: %v", v.Event, err)
		}
	}
	joinClient := providerEvent("join-client", "participant_joined", identityClient, roomID, start)
	joinSpecialist := providerEvent("join-specialist", "participant_joined", identitySpecialist, roomID, start.Add(time.Second))
	audioClient := providerEvent("audio-client", "track_published", identityClient, roomID, start.Add(2*time.Second))
	audioClient.Track.Type = "AUDIO"
	audioSpecialist := providerEvent("audio-specialist", "track_published", identitySpecialist, roomID, start.Add(3*time.Second))
	audioSpecialist.Track.Type = "AUDIO"
	apply(joinClient)
	apply(joinSpecialist)
	apply(audioClient)
	room, err := s.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if room.Started || len(room.Outbox) != 2 {
		t.Fatal("one-sided media incorrectly caused STARTED")
	}
	apply(audioSpecialist)
	apply(audioSpecialist)
	room, err = s.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if !room.Started || len(room.Outbox) != 3 || room.Outbox[2].Sequence != 3 {
		t.Fatalf("both participant audio tracks did not give one STARTED evidence: %+v", room)
	}
	for _, token := range []string{tokenClient, tokenSpecialist} {
		req := httptest.NewRequest(http.MethodPost, "/v1/rooms/finish", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.handleFinish(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("finish intent denied: %d %s", rec.Code, rec.Body.String())
		}
	}
	apply(providerEvent("left-client", "participant_left", identityClient, roomID, start.Add(85*time.Second)))
	apply(providerEvent("left-specialist", "participant_left", identitySpecialist, roomID, start.Add(86*time.Second)))
	apply(providerEvent("room-finished", "room_finished", "", roomID, start.Add(89*time.Second)))
	room, err = s.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if !room.Terminal || room.Outbox[len(room.Outbox)-1].Payload == nil {
		t.Fatal("mutually-ended room did not create terminal evidence")
	}
	var terminal struct {
		FactType string `json:"fact_type"`
	}
	if json.Unmarshal(room.Outbox[len(room.Outbox)-1].Payload, &terminal) != nil || terminal.FactType != "ENDED" {
		t.Fatalf("incorrect terminal event: %+v", terminal)
	}
	if room.Delivered != 0 {
		t.Fatal("events wrongly marked delivered without APGIC ACK")
	}
	restart := newReferenceService(s.config)
	restart.clock = s.clock
	var received []signedFact
	restart.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(req.Body)
		var body signedFact
		if json.Unmarshal(raw, &body) != nil {
			t.Fatal("bridge submitted malformed envelope")
		}
		var signing factForSigning
		signing.ConnectorInstanceID = body.ConnectorInstanceID
		signing.ExternalEventID = body.ExternalEventID
		signing.StreamID = body.StreamID
		signing.Sequence = body.Sequence
		signing.KeyID = body.KeyID
		signing.OccurredAt = body.OccurredAt
		signing.Payload = body.Payload
		bytesToSign, _ := json.Marshal(signing)
		sig, e := base64.StdEncoding.DecodeString(body.Signature)
		if e != nil || !ed25519.Verify(s.config.SigningKey.Public().(ed25519.PublicKey), bytesToSign, sig) {
			t.Fatal("unsigned or incorrectly signed consultation fact")
		}
		received = append(received, body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"delivery_decision":"APPLY"}`)), Header: make(http.Header)}, nil
	})}
	restart.flushRoom(booking)
	check, err := restart.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if check.Delivered != len(check.Outbox) || len(received) != len(check.Outbox) || len(received) < 6 {
		t.Fatalf("restarted bridge failed to replay signed evidence: sent=%d all=%d delivered=%d",
			len(received), len(check.Outbox), check.Delivered)
	}
	for i, item := range received {
		if item.Sequence != uint64(i+1) {
			t.Fatalf("out-of-order provider stream %v", item.Sequence)
		}
	}
	restart.flushRoom(booking)
	if len(received) != len(check.Outbox) {
		t.Fatal("outbox replay created duplicates after ACK")
	}
}

func TestUnilateralExitIsFailureNotCompletion(t *testing.T) {
	s, start := referenceFixture(t)
	id := "55555555-5555-4555-8555-555555555555"
	c := "66666666-6666-4666-8666-666666666666"
	p := "77777777-7777-4777-8777-777777777777"
	issueFixture(t, s, id, c, "CLIENT", "client-key-0000000000001", start.Add(5*time.Minute))
	issueFixture(t, s, id, p, "SPECIALIST", "specialist-key-000000001", start.Add(5*time.Minute))
	ic, ip := identityFor("CLIENT", c), identityFor("SPECIALIST", p)
	mutate := func(event livekitEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		room, e := s.readRoom(id)
		if e == nil {
			e = s.applyLiveKitEvent(room, event)
		}
		if e == nil {
			e = s.writeRoom(room)
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	roomID := roomName(id)
	mutate(providerEvent("j1", "participant_joined", ic, roomID, start))
	mutate(providerEvent("j2", "participant_joined", ip, roomID, start.Add(time.Second)))
	a := providerEvent("a1", "track_published", ic, roomID, start.Add(time.Second))
	a.Track.Type = "AUDIO"
	mutate(a)
	b := providerEvent("a2", "track_published", ip, roomID, start.Add(2*time.Second))
	b.Track.Type = "AUDIO"
	mutate(b)
	mutate(providerEvent("l1", "participant_left", ic, roomID, start.Add(75*time.Second)))
	mutate(providerEvent("finish", "room_finished", "", roomID, start.Add(85*time.Second)))
	state, e := s.readRoom(id)
	if e != nil {
		t.Fatal(e)
	}
	var terminal struct {
		FactType string `json:"fact_type"`
	}
	if json.Unmarshal(state.Outbox[len(state.Outbox)-1].Payload, &terminal) != nil || terminal.FactType != "TECHNICAL_FAILURE" {
		t.Fatalf("one party's disconnection was misreported as consultation success: %+v", terminal)
	}
}

func TestLiveKitSignedWebhookRejectsPayloadTampering(t *testing.T) {
	s,start:=referenceFixture(t)
	body:=[]byte(`{"id":"livekit-event","event":"room_started"}`)
	hash:=sha256.Sum256(body)
	claims:=map[string]any{
		"iss":s.config.APIKey,
		"exp":start.Add(time.Minute).Unix(),
		"nbf":start.Unix()-5,
		"sha256":base64.StdEncoding.EncodeToString(hash[:]),
	}
	claimsBytes,err:=json.Marshal(claims)
	if err!=nil {t.Fatal(err)}
	input:=base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))+"."+
		base64.RawURLEncoding.EncodeToString(claimsBytes)
	mac:=hmac.New(sha256.New,[]byte(s.config.APISecret))
	_,_=mac.Write([]byte(input))
	token:=input+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if err:=s.verifyLiveKitWebhook(body,token);err!=nil {
		t.Fatalf("provider signed webhook rejected: %v",err)
	}
	if s.verifyLiveKitWebhook([]byte(`{"id":"tampered","event":"room_started"}`),token)==nil {
		t.Fatal("tampered provider body passed the signed SHA-256 check")
	}
	if s.verifyLiveKitWebhook(body,token[:len(token)-3])==nil {
		t.Fatal("corrupted LiveKit JWT signature accepted")
	}
	if s.verifyLiveKitWebhook(body,"")==nil {
		t.Fatal("anonymous provider webhook accepted")
	}
}
