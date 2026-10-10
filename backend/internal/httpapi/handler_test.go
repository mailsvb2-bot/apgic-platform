package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
)

func readyConfig() launchconfig.Config {
	return launchconfig.Config{
		JurisdictionMatrixVersion: "jurisdiction-ci-v1",
		RetentionPolicyVersion:    "retention-ci-v1",
		SLOPolicyVersion:          "slo-ci-v1",
		ProviderMatrixVersion:     "providers-ci-v1",
	}
}

func TestPublicProviderEventCannotSelfAttestPayment(t *testing.T) {
	handler := New(Options{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/provider-events", strings.NewReader(`{"outcome":"CAPTURED","provider_id":"external-bank","provider_event_id":"forged","order_id":"forged","amount_minor":100,"currency":"RUB"}`)))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("browser payment attestation status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !contains(recorder.Body.String(), "PROVIDER_EVIDENCE_UNVERIFIED") {
		t.Fatalf("expected fail-closed provider boundary: %s", recorder.Body.String())
	}
	meta := httptest.NewRecorder()
	handler.ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/v1/meta", nil))
	if contains(meta.Body.String(), "\"conformance_provider_events\":true") {
		t.Fatalf("default runtime must not advertise synthetic captures: %s", meta.Body.String())
	}
}

func TestPublicCheckoutFailsClosedWithoutConfiguredExternalProvider(t *testing.T) {
	handler := New(Options{Demand: demand.NewConformanceService(nil)})

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/v1/slot-holds/forged/checkout-options", nil))
	if options.Code != http.StatusServiceUnavailable || !contains(options.Body.String(), "PAYMENT_PROVIDER_UNAVAILABLE") {
		t.Fatalf("checkout options must fail closed without provider: status=%d body=%s", options.Code, options.Body.String())
	}

	instruction := httptest.NewRecorder()
	handler.ServeHTTP(instruction, httptest.NewRequest(http.MethodPost, "/v1/checkout-instructions", strings.NewReader(`{"hold_id":"forged","method_code":"BANK_CARD"}`)))
	if instruction.Code != http.StatusServiceUnavailable || !contains(instruction.Body.String(), "PAYMENT_PROVIDER_UNAVAILABLE") {
		t.Fatalf("checkout instruction must fail closed without provider: status=%d body=%s", instruction.Code, instruction.Body.String())
	}
}

type providerWebhookKeyMap map[string]ed25519.PublicKey

func (m providerWebhookKeyMap) ResolveWebhookPublicKey(connectorInstanceID, keyID string) (ed25519.PublicKey, bool) {
	key, ok := m[connectorInstanceID+"/"+keyID]
	return key, ok
}

func signProviderWebhook(t *testing.T, envelope connector.WebhookEnvelope, privateKey ed25519.PrivateKey) connector.WebhookEnvelope {
	t.Helper()
	message, err := json.Marshal(struct {
		ConnectorInstanceID string          `json:"connector_instance_id"`
		ExternalEventID     string          `json:"external_event_id"`
		StreamID            string          `json:"stream_id"`
		Sequence            uint64          `json:"sequence"`
		KeyID               string          `json:"key_id"`
		OccurredAt          time.Time       `json:"occurred_at"`
		Payload             json.RawMessage `json:"payload"`
	}{
		ConnectorInstanceID: envelope.ConnectorInstanceID,
		ExternalEventID:     envelope.ExternalEventID,
		StreamID:            envelope.StreamID,
		Sequence:            envelope.Sequence,
		KeyID:               envelope.KeyID,
		OccurredAt:          envelope.OccurredAt,
		Payload:             envelope.Payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	return envelope
}

func TestTrustedProviderWebhookRequiresSignatureAndConfirmsExactlyOnce(t *testing.T) {
	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const connectorID = "payment-connector-sandbox"
	const keyID = "provider-key-v1"
	eventID := "provider-event-signed"
	payload, err := json.Marshal(providerEventRequest{
		ProviderID:      instruction.ProviderID,
		ProviderEventID: eventID,
		OrderID:         instruction.OrderID,
		AmountMinor:     instruction.AmountMinor,
		Currency:        instruction.Currency,
		Outcome:         "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := signProviderWebhook(t, connector.WebhookEnvelope{
		ConnectorInstanceID: connectorID,
		ExternalEventID:     eventID,
		StreamID:            "payment/" + instruction.OrderID,
		Sequence:            1,
		KeyID:               keyID,
		OccurredAt:          time.Date(2026, 10, 9, 7, 0, 0, 0, time.UTC),
		Payload:             payload,
	}, privateKey)

	handler := New(Options{
		Demand:              service,
		ProviderWebhookKeys: providerWebhookKeyMap{connectorID + "/" + keyID: publicKey},
	})
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/v1/provider-webhooks", strings.NewReader(string(encoded))))
	if first.Code != http.StatusCreated || !contains(first.Body.String(), "\"booking_state\":\"CONFIRMED\"") {
		t.Fatalf("signed provider webhook status=%d body=%s", first.Code, first.Body.String())
	}

	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, httptest.NewRequest(http.MethodPost, "/v1/provider-webhooks", strings.NewReader(string(encoded))))
	if replay.Code != http.StatusOK || !contains(replay.Body.String(), "\"idempotent\":true") {
		t.Fatalf("signed provider webhook replay status=%d body=%s", replay.Code, replay.Body.String())
	}

	forgedEnvelope := envelope
	forgedEnvelope.Payload = append(json.RawMessage(nil), envelope.Payload...)
	var forged providerEventRequest
	if err := json.Unmarshal(forgedEnvelope.Payload, &forged); err != nil {
		t.Fatal(err)
	}
	forged.AmountMinor++
	forgedEnvelope.Payload, _ = json.Marshal(forged)
	forgedEncoded, _ := json.Marshal(forgedEnvelope)
	forgedResponse := httptest.NewRecorder()
	handler.ServeHTTP(forgedResponse, httptest.NewRequest(http.MethodPost, "/v1/provider-webhooks", strings.NewReader(string(forgedEncoded))))
	if forgedResponse.Code != http.StatusUnauthorized || !contains(forgedResponse.Body.String(), "PROVIDER_WEBHOOK_UNVERIFIED") {
		t.Fatalf("tampered signed webhook status=%d body=%s", forgedResponse.Code, forgedResponse.Body.String())
	}
}

func TestTrustedProviderWebhookFailsClosedWithoutConfiguredProviderKey(t *testing.T) {
	handler := New(Options{Demand: demand.NewConformanceService(nil)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/provider-webhooks", strings.NewReader(`{"connector_instance_id":"payment","external_event_id":"evt","stream_id":"payment/order","sequence":1,"key_id":"k","occurred_at":"2026-10-09T07:00:00Z","payload":{"provider_id":"external-bank"},"signature":"ZmFrZQ=="}`)))
	if recorder.Code != http.StatusUnauthorized || !contains(recorder.Body.String(), "PROVIDER_WEBHOOK_UNVERIFIED") {
		t.Fatalf("unconfigured provider webhook status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthDoesNotPretendToBeReadiness(t *testing.T) {
	handler := New(Options{})

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", ready.Code, http.StatusServiceUnavailable)
	}
}

func TestReadinessRequiresExplicitVersionedLaunchConfig(t *testing.T) {
	handler := New(Options{LaunchConfig: readyConfig()})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessFailsClosedWhenStorageIsUnavailable(t *testing.T) {
	handler := New(Options{
		LaunchConfig: readyConfig(),
		ReadinessCheck: func(context.Context) error {
			return errors.New("database unavailable")
		},
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !contains(recorder.Body.String(), "STORAGE_UNAVAILABLE") {
		t.Fatalf("readiness reason missing: %s", recorder.Body.String())
	}
}

func TestRuntimeSLIExposesLatencyStatusAndReadinessReasonWithoutRawPathIDs(t *testing.T) {
	handler := New(Options{
		LaunchConfig: readyConfig(),
		ReadinessCheck: func(context.Context) error {
			return errors.New("database unavailable")
		},
	})

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d", ready.Code)
	}

	unmatched := httptest.NewRecorder()
	handler.ServeHTTP(unmatched, httptest.NewRequest(http.MethodGet, "/v1/private-object/secret-identifier", nil))
	if unmatched.Code != http.StatusNotFound {
		t.Fatalf("unmatched status = %d", unmatched.Code)
	}

	metrics := httptest.NewRecorder()
	handler.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("metrics status = %d body=%s", metrics.Code, metrics.Body.String())
	}
	body := metrics.Body.String()
	for _, expected := range []string{
		"apgic_http_requests_total",
		"apgic_http_request_duration_milliseconds_bucket",
		"route=\"/readyz\"",
		"status=\"503\"",
		"apgic_readiness_checks_total{result=\"not_ready\",reason=\"STORAGE_UNAVAILABLE\"} 1",
		"route=\"UNMATCHED\"",
	} {
		if !contains(body, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, body)
		}
	}
	if contains(body, "secret-identifier") {
		t.Fatalf("metrics leaked raw request path identifier: %s", body)
	}
}

func TestMetaIsStableAndEvidenceBearing(t *testing.T) {
	now := time.Date(2026, 9, 23, 17, 0, 0, 0, time.UTC)
	handler := New(Options{
		CommitSHA:    "abc123",
		LaunchConfig: readyConfig(),
		Now:          func() time.Time { return now },
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/meta", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("meta status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{"abc123", "2026-09-23T17:00:00Z", "IOS", "ANDROID"} {
		if !contains(body, expected) {
			t.Fatalf("meta response missing %q: %s", expected, body)
		}
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}


func TestUntrustedConsultationLifecycleWritesFailClosed(t *testing.T) {
	handler := New(Options{Demand: demand.NewConformanceService(nil)})
	for _, route := range []string{
		"/v1/consultations/forged/presence",
		"/v1/consultations/forged/failures",
		"/v1/consultations/forged/recovery",
		"/v1/consultations/forged/complete",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, route, strings.NewReader("{}")))
		if response.Code != http.StatusForbidden || !contains(response.Body.String(), "CONSULT_PROVIDER_EVIDENCE_UNVERIFIED") {
			t.Fatalf("%s accepted untrusted provider lifecycle data: status=%d body=%s", route, response.Code, response.Body.String())
		}
	}
}
