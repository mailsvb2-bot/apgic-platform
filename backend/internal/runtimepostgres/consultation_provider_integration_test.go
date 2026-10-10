package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// Run only against an isolated, migrated PostgreSQL integration database.
// An external provider is simulated cryptographically; no funds move.
func TestSignedConsultationPersistsAcrossAPIStoreRestart(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CONNECTOR_DELIVERY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated connector integration database is not configured")
	}
	ctx := context.Background()
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil {
		t.Fatal(err)
	}
	// The browser obtains a real signed session cookie from the first API handler.
	sessionKey := []byte("isolated-integration-client-session-key-123")
	firstAPI := httpapi.New(httpapi.Options{Demand: service, ClientSessionKey: sessionKey})
	entry := httptest.NewRecorder()
	firstAPI.ServeHTTP(entry, httptest.NewRequest(http.MethodPost, "/v1/help-intents",
		strings.NewReader(`{"free_text":"бессонница"}`)))
	if entry.Code != http.StatusCreated {
		t.Fatalf("entry status=%d body=%s", entry.Code, entry.Body.String())
	}
	var intent demand.Intent
	if err := json.Unmarshal(entry.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range entry.Result().Cookies() {
		if cookie.Name == "__Host-apgic_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("trusted browser session was not issued")
	}
	if _, err = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%d error=%v", len(slots), err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	// Represents a prior trusted payment-provider capture in the isolated fixture.
	if _, err = store.db.ExecContext(ctx,
		"UPDATE bookings SET state=$1,updated_at=now() WHERE id=$2::uuid", booking.StateConfirmed, hold.BookingID); err != nil {
		t.Fatal(err)
	}
	connectorID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx,
		`INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
          VALUES($1::uuid,'COMMUNICATION_PROVIDER','consultation-integration','ACTIVE',$2)`,
		connectorID, "secretref://consultation/"+connectorID); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := connectorWebhookKeyMap{connectorID + "/test-key": public}
	providerAPI := httpapi.New(httpapi.Options{ConsultationProvider: store, ProviderWebhookKeys: keys, ClientSessionKey: sessionKey})
	postSigned := func(event connector.WebhookEnvelope) (int, connector.DeliveryDecision) {
		body, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		response := httptest.NewRecorder()
		providerAPI.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/consultation-provider-webhooks", strings.NewReader(string(body))))
		var parsed struct {
			DeliveryDecision string `json:"delivery_decision"`
		}
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
				t.Fatal(err)
			}
		}
		return response.Code, connector.DeliveryDecision(parsed.DeliveryDecision)
	}
	now := time.Now().UTC().Truncate(time.Microsecond).Add(time.Minute)
	signed := func(seq uint64, factType, role string) connector.WebhookEnvelope {
		payload, e := json.Marshal(ConsultationProviderEvent{
			BookingID: hold.BookingID, FactType: factType, Role: role,
			EvidenceRef:       "provider-evidence/" + factType + "/" + role,
			ProviderReference: "provider-room/" + hold.BookingID,
		})
		if e != nil {
			t.Fatal(e)
		}
		raw := connectorWebhookSigningPayload{
			ConnectorInstanceID: connectorID,
			ExternalEventID:     "consultation-event-" + hold.BookingID + "-" + factType + "-" + role,
			StreamID:            "consultation/" + hold.BookingID,
			Sequence:            seq, KeyID: "test-key", OccurredAt: now.Add(time.Duration(seq) * time.Second), Payload: payload,
		}
		message, e := json.Marshal(raw)
		if e != nil {
			t.Fatal(e)
		}
		return connector.WebhookEnvelope{
			ConnectorInstanceID: raw.ConnectorInstanceID, ExternalEventID: raw.ExternalEventID,
			StreamID: raw.StreamID, Sequence: raw.Sequence, KeyID: raw.KeyID, OccurredAt: raw.OccurredAt,
			Payload: raw.Payload, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, message)),
		}
	}
	for _, event := range []connector.WebhookEnvelope{
		signed(1, "JOINED", "CLIENT"), signed(2, "JOINED", "SPECIALIST"),
		signed(3, "STARTED", "SYSTEM"), signed(4, "ENDED", "SYSTEM"),
	} {
		if event.Sequence == 1 {
			forged := event
			forged.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			if status, _ := postSigned(forged); status != http.StatusUnauthorized {
				t.Fatalf("forged provider signature accepted: HTTP %d", status)
			}
		}
		status, decision := postSigned(event)
		if status != http.StatusOK || decision != connector.DeliveryApply {
			t.Fatalf("event %d HTTP=%d decision=%s", event.Sequence, status, decision)
		}
		if event.Sequence == 4 {
			status, decision = postSigned(event)
			if status != http.StatusOK || decision != connector.DeliveryDuplicate {
				t.Fatalf("replay HTTP=%d decision=%s", status, decision)
			}
		}
	}
	// The next Checker is a separate connection, equivalent to restarting API
	// without any in-process consultation sessions.
	restarted, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	result, err := restarted.ReadConsultationResult(ctx, hold.BookingID, intent.ClientIdentityID)
	if err != nil || result == nil || result.State != "COMPLETED" || result.CompletionEvidenceRef != "provider-evidence/ENDED/SYSTEM" {
		t.Fatalf("restart result=%#v err=%v", result, err)
	}
	stranger, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	unauthorized, err := restarted.ReadConsultationResult(ctx, hold.BookingID, stranger)
	if err != nil || unauthorized != nil {
		t.Fatalf("other client result=%#v err=%v", unauthorized, err)
	}
	var count int
	if err = restarted.db.QueryRowContext(ctx,
		"SELECT count(*) FROM consultation_lifecycle_facts f JOIN consultation_sessions s ON s.id=f.session_id WHERE s.booking_id=$1::uuid AND f.fact_type='ENDED'", hold.BookingID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicated completion facts: %d", count)
	}

	// Reconstruct HTTP API against a separate PostgreSQL connection. Nothing
	// about the consultation outcome comes from the original API process memory.
	secondAPI := httpapi.New(httpapi.Options{ConsultationProvider: restarted, ClientSessionKey: sessionKey})
	request := httptest.NewRequest(http.MethodGet, "/v1/consultations/"+hold.BookingID+"/result", nil)
	request.AddCookie(sessionCookie)
	readback := httptest.NewRecorder()
	secondAPI.ServeHTTP(readback, request)
	if readback.Code != http.StatusOK {
		t.Fatalf("new HTTP API cannot read persisted result: HTTP=%d body=%s", readback.Code, readback.Body.String())
	}
	var apiResult connector.ConsultationResult
	if err := json.Unmarshal(readback.Body.Bytes(), &apiResult); err != nil {
		t.Fatal(err)
	}
	if apiResult.State != "COMPLETED" || apiResult.CompletionEvidenceRef != "provider-evidence/ENDED/SYSTEM" {
		t.Fatalf("wrong HTTP restart result: %#v", apiResult)
	}
	outsiderRead := httptest.NewRecorder()
	secondAPI.ServeHTTP(outsiderRead, httptest.NewRequest(http.MethodGet, "/v1/consultations/"+hold.BookingID+"/result", nil))
	if outsiderRead.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated read HTTP=%d body=%s", outsiderRead.Code, outsiderRead.Body.String())
	}
}
