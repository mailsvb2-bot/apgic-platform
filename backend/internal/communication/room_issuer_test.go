package communication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderRoomIssuerIssuesScopedGrantAndRejectsForeignRooms(t *testing.T) {
	expiry := time.Now().UTC().Add(4 * time.Minute).Truncate(time.Second)
	var roomOrigin atomic.Value
	roomOrigin.Store("https://media.example")
	var requests atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("X-APGIC-Contract") != "room-join-v1" ||
			r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 40) {
			t.Errorf("issuer request was not authenticated/versioned: %+v", r.Header)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var command map[string]any
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
			t.Error(err)
		}
		if command["contract_version"] != "apgic.room-join.v1" ||
			command["scope"] != "ROOM_JOIN" || command["role"] != "CLIENT" {
			t.Errorf("provider received invalid scope: %+v", command)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(IssuedRoomGrant{
			ProviderCredentialRef: "opaque/credential-1",
			JoinURI:               roomOrigin.Load().(string) + "/call?token=transient",
			ExpiresAt:             expiry.Add(-time.Second),
		})
	}))
	defer server.Close()
	issuer, err := NewHTTPRoomIssuer(server.URL, strings.Repeat("s", 40), "https://media.example")
	if err != nil {
		t.Fatal(err)
	}
	issuer.client = server.Client()
	command := CredentialRequest{
		ProviderInstanceID: "provider-1", BookingID: "booking-1",
		IdentityID: "client-1", Role: RoleClient, Scope: "ROOM_JOIN",
		IdempotencyKey: "join-idempotency-0001", ExpiresAt: expiry,
	}
	issued, err := issuer.IssueRoomJoin(context.Background(), command)
	if err != nil || issued.ProviderCredentialRef != "opaque/credential-1" ||
		issued.JoinURI != "https://media.example/call?token=transient" {
		t.Fatalf("scoped grant failed: %+v %v", issued, err)
	}
	roomOrigin.Store("https://evil.example")
	if _, err := issuer.IssueRoomJoin(context.Background(), command); !errors.Is(err, ErrRoomGrantInvalid) {
		t.Fatalf("cross-origin grant accepted: %v", err)
	}
	bad := command
	bad.Scope = "FULL_ACCESS"
	if _, err := issuer.IssueRoomJoin(context.Background(), bad); !errors.Is(err, ErrRoomIssuerUnavailable) {
		t.Fatalf("unsafe scope accepted: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("unsafe request unexpectedly dispatched: %d", requests.Load())
	}
	if _, err := NewHTTPRoomIssuer("http://insecure.example", strings.Repeat("s", 40), "https://media.example"); err == nil {
		t.Fatal("untrusted HTTP room issuer configured")
	}
	if _, err := NewHTTPRoomIssuer(server.URL, "short", "https://media.example"); err == nil {
		t.Fatal("insufficient server credential accepted")
	}
}

func TestProviderRoomIssuerWillNotFollowRedirects(t *testing.T) {
	var targetVisited atomic.Bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetVisited.Store(true)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	issuer, err := NewHTTPRoomIssuer(source.URL, strings.Repeat("s", 40), "https://media.example")
	if err != nil {
		t.Fatal(err)
	}
	issuer.client.Transport = source.Client().Transport
	request := CredentialRequest{
		ProviderInstanceID: "provider-1", BookingID: "booking-1",
		IdentityID: "client-1", Role: RoleClient, Scope: "ROOM_JOIN",
		IdempotencyKey: "join-idempotency-0002", ExpiresAt: time.Now().Add(time.Minute),
	}
	if _, err := issuer.IssueRoomJoin(context.Background(), request); !errors.Is(err, ErrRoomIssuerUnavailable) {
		t.Fatalf("provider redirect was followed: %v", err)
	}
	if targetVisited.Load() {
		t.Fatal("room bearer credential was forwarded to redirected host")
	}
}
