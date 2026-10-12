package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	// Unlike an artificial direct UPDATE of bookings, the payment boundary is
	// exercised end-to-end: signed PSP webhook -> canonical payment evidence ->
	// persistent economic ledger -> PostgreSQL CONFIRMED booking.
	checkout, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	if checkout.APGICAcceptsFunds || checkout.ExecutionOwner != "EXTERNAL_PROVIDER" {
		t.Fatalf("payment custody is not external: %#v", checkout)
	}
	// Payment connector keys are provider-scoped: only the provider named
	// in the immutable checkout may sign its economic CAPTURED event.
	paymentConnectorID := checkout.ProviderID
	paymentPublic, paymentPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	paymentKeys := connectorWebhookKeyMap{paymentConnectorID + "/payment-key": paymentPublic}
	paymentAPI := httpapi.New(httpapi.Options{
		Demand: service, ProviderWebhookKeys: paymentKeys, ClientSessionKey: sessionKey,
	})
	paymentEventID := "payment-capture-" + hold.BookingID
	paymentPayload, err := json.Marshal(map[string]any{
		"provider_id":       checkout.ProviderID,
		"provider_event_id": paymentEventID,
		"order_id":          checkout.OrderID,
		"amount_minor":      checkout.AmountMinor,
		"currency":          checkout.Currency,
		"outcome":           "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	paymentEnvelope := connector.WebhookEnvelope{
		ConnectorInstanceID: paymentConnectorID,
		ExternalEventID:     paymentEventID,
		StreamID:            "payment/" + checkout.OrderID,
		Sequence:            1,
		KeyID:               "payment-key",
		OccurredAt:          time.Now().UTC(),
		Payload:             paymentPayload,
	}
	paymentRaw := connectorWebhookSigningPayload{
		ConnectorInstanceID: paymentEnvelope.ConnectorInstanceID,
		ExternalEventID:     paymentEnvelope.ExternalEventID,
		StreamID:            paymentEnvelope.StreamID,
		Sequence:            paymentEnvelope.Sequence,
		KeyID:               paymentEnvelope.KeyID,
		OccurredAt:          paymentEnvelope.OccurredAt,
		Payload:             paymentEnvelope.Payload,
	}
	paymentBytes, err := json.Marshal(paymentRaw)
	if err != nil {
		t.Fatal(err)
	}
	postPayment := func(event connector.WebhookEnvelope) (int, string) {
		body, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		response := httptest.NewRecorder()
		paymentAPI.ServeHTTP(response,
			httptest.NewRequest(http.MethodPost, "/v1/provider-webhooks", strings.NewReader(string(body))))
		return response.Code, response.Body.String()
	}
	forgedPayment := paymentEnvelope
	forgedPayment.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if status, body := postPayment(forgedPayment); status != http.StatusUnauthorized {
		t.Fatalf("forged payment accepted: HTTP %d body=%s", status, body)
	}
	paymentEnvelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(paymentPrivate, paymentBytes))
	if status, body := postPayment(paymentEnvelope); status != http.StatusCreated {
		t.Fatalf("signed payment capture rejected: HTTP %d body=%s", status, body)
	}
	if status, body := postPayment(paymentEnvelope); status != http.StatusOK {
		t.Fatalf("replayed payment was not idempotent: HTTP %d body=%s", status, body)
	}
	confirmed, err := service.HoldStatus(hold.ID, intent.ClientIdentityID)
	if err != nil || confirmed.BookingState != booking.StateConfirmed {
		t.Fatalf("signed payment did not confirm booking: hold=%#v err=%v", confirmed, err)
	}
	var canonicalState string
	if err := store.db.QueryRowContext(ctx,
		"SELECT state FROM bookings WHERE id=$1::uuid", hold.BookingID).Scan(&canonicalState); err != nil {
		t.Fatal(err)
	}
	if canonicalState != string(booking.StateConfirmed) {
		t.Fatalf("PostgreSQL booking state=%s, expected CONFIRMED", canonicalState)
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
			ExternalEventID:     "consultation-event-" + hold.BookingID + "-" + factType + "-" + role + "-" + strconv.FormatUint(seq, 10),
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
	signAs := func(event connector.WebhookEnvelope, provider string, sequence uint64, key ed25519.PrivateKey) connector.WebhookEnvelope {
		event.ConnectorInstanceID = provider
		event.Sequence = sequence
		raw := connectorWebhookSigningPayload{
			ConnectorInstanceID: event.ConnectorInstanceID,
			ExternalEventID:     event.ExternalEventID,
			StreamID:            event.StreamID,
			Sequence:            event.Sequence,
			KeyID:               event.KeyID,
			OccurredAt:          event.OccurredAt,
			Payload:             event.Payload,
		}
		message, marshalErr := json.Marshal(raw)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		event.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, message))
		return event
	}

	// A privileged/stale booking flag cannot substitute for a signed PSP
	// capture. Prove this using a second PostgreSQL booking whose state was
	// set CONFIRMED without any captured payment ledger entry. The webhook
	// itself has a VALID provider signature; only economic authorization
	// should reject it and roll back the connector business effect.
	unpaidIntent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(unpaidIntent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	unpaidSlots, err := service.Slots("spec-lebedeva")
	if err != nil || len(unpaidSlots) < 1 {
		t.Fatalf("need second distinct slot to prove unpaid access: %d %v", len(unpaidSlots), err)
	}
	unpaidHold, err := service.AcquireHold(unpaidIntent.ID, unpaidSlots[0].ID, unpaidIntent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCheckout(unpaidHold.ID, unpaidIntent.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE bookings SET state='CONFIRMED', updated_at=now()
		  WHERE id=$1::uuid AND state='PENDING_PAYMENT'`, unpaidHold.BookingID); err != nil {
		t.Fatalf("forge booking without capture: %v", err)
	}
	forgedEntitlement := signed(1, "JOINED", "CLIENT")
	forgedPayload, err := json.Marshal(ConsultationProviderEvent{
		BookingID:         unpaidHold.BookingID,
		FactType:          "JOINED",
		Role:              "CLIENT",
		ProviderReference: "unpaid-attempt/" + unpaidHold.BookingID,
		EvidenceRef:       "signed-but-unpaid/" + unpaidHold.BookingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	forgedEntitlement.Payload = forgedPayload
	forgedEntitlement.StreamID = "consultation/" + unpaidHold.BookingID
	forgedEntitlement.ExternalEventID = "unpaid-" + unpaidHold.BookingID
	forgedEntitlement = signAs(forgedEntitlement, connectorID, 1, private)
	if status, _ := postSigned(forgedEntitlement); status != http.StatusConflict {
		t.Fatalf("valid signed communication webhook opened UNPAID confirmed booking: HTTP %d", status)
	}
	// Replaying a refused event cannot claim success or mint a receipt.
	if status, _ := postSigned(forgedEntitlement); status != http.StatusConflict {
		t.Fatalf("replayed unpaid signed event was accepted: HTTP %d", status)
	}
	unpaidOrder, err := service.ReadOrderForBooking(unpaidHold.BookingID, unpaidIntent.ClientIdentityID)
	if err != nil || unpaidOrder == nil || unpaidOrder.BookingState != booking.StateConfirmed ||
		unpaidOrder.PaymentState != "PENDING_PROVIDER" || unpaidOrder.RefundState != "" {
		t.Fatalf("unpaid CONFIRMED booking obscured missing external capture: %#v err=%v", unpaidOrder, err)
	}
	var forgedSessions int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM consultation_sessions WHERE booking_id=$1::uuid",
		unpaidHold.BookingID).Scan(&forgedSessions); err != nil {
		t.Fatal(err)
	}
	if forgedSessions != 0 {
		t.Fatalf("unpaid booking generated %d durable consultation sessions", forgedSessions)
	}
	if read, err := store.ReadConsultationResult(ctx, unpaidHold.BookingID, unpaidIntent.ClientIdentityID); err != nil || read != nil {
		t.Fatalf("unpaid booking yielded consultation result: %#v err=%v", read, err)
	}

	var fallbackID string
	var fallbackPrivate ed25519.PrivateKey
	for _, original := range []connector.WebhookEnvelope{
		signed(1, "JOINED", "CLIENT"), signed(2, "JOINED", "SPECIALIST"),
		signed(3, "STARTED", "SYSTEM"),
		signed(4, "JOINED", "CLIENT"), // delayed rejoin cannot rewind an active call
		signed(5, "LEFT", "CLIENT"),
		signed(6, "RECOVERY_STARTED", "SYSTEM"),
		signed(7, "RECOVERY_SUCCEEDED", "SYSTEM"),
		signed(8, "ENDED", "SYSTEM"),
	} {
		stage := original.Sequence
		event := original
		if stage >= 7 {
			// A distinct connector owns its own monotonically sequenced stream.
			if fallbackID == "" || len(fallbackPrivate) == 0 {
				t.Fatal("fallback provider was not authorized before its first event")
			}
			event = signAs(original, fallbackID, stage-6, fallbackPrivate)
		}
		if stage == 1 {
			forged := event
			forged.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			if status, _ := postSigned(forged); status != http.StatusUnauthorized {
				t.Fatalf("forged provider signature accepted: HTTP %d", status)
			}
		}
		status, decision := postSigned(event)
		if status != http.StatusOK || decision != connector.DeliveryApply {
			t.Fatalf("event %d (provider=%s) HTTP=%d decision=%s", stage, event.ConnectorInstanceID, status, decision)
		}
		if stage == 4 || stage == 6 || stage == 7 {
			intermediate, readErr := store.ReadConsultationResult(ctx, hold.BookingID, intent.ClientIdentityID)
			want := "IN_PROGRESS"
			if stage == 6 {
				want = "RECOVERING"
			}
			if readErr != nil || intermediate == nil || intermediate.State != want {
				t.Fatalf("provider event %d: state=%#v err=%v (want %s)", stage, intermediate, readErr, want)
			}
		}
		if stage == 6 {
			// Reviewer's P1: a signed fallback event must fail before, and
			// succeed only after, an immutable canonical recovery decision.
			fallbackID, err = persistentid.New()
			if err != nil {
				t.Fatal(err)
			}
			fallbackPublic, privateKey, keyErr := ed25519.GenerateKey(nil)
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			fallbackPrivate = privateKey
			keys[fallbackID+"/test-key"] = fallbackPublic
			if _, err := store.db.ExecContext(ctx,
				`INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
				 VALUES($1::uuid,'COMMUNICATION_PROVIDER','consultation-fallback','ACTIVE',$2)`,
				fallbackID, "secretref://consultation/fallback/"+fallbackID); err != nil {
				t.Fatal(err)
			}
			notYetAuthorized := signAs(signed(7, "RECOVERY_SUCCEEDED", "SYSTEM"), fallbackID, 1, fallbackPrivate)
			if rejectedStatus, _ := postSigned(notYetAuthorized); rejectedStatus != http.StatusConflict {
				t.Fatalf("fallback without trusted recovery decision accepted: HTTP %d", rejectedStatus)
			}
			var sessionID, sourceFactID string
			if err := store.db.QueryRowContext(ctx,
				`SELECT session_id::text, id::text FROM consultation_lifecycle_facts
				 WHERE idempotency_key=$1`, event.ExternalEventID).Scan(&sessionID, &sourceFactID); err != nil {
				t.Fatal(err)
			}
			decisionID, idErr := persistentid.New()
			if idErr != nil {
				t.Fatal(idErr)
			}
			if _, err := store.db.ExecContext(ctx,
				`INSERT INTO consultation_recovery_decisions
				 (id,session_id,source_fact_id,policy_version,action,target_provider_instance_id,
				  followup_path_ref,reason_code,decided_at)
				 VALUES($1::uuid,$2::uuid,$3::uuid,'r3-signed-integration-v1','FALLBACK_PROVIDER',
				        $4::uuid,NULL,'COMM_RECOVERY_FALLBACK',$5)`,
				decisionID, sessionID, sourceFactID, fallbackID, event.OccurredAt.Add(time.Millisecond)); err != nil {
				t.Fatal(err)
			}
		}
		if stage == 8 {
			status, decision = postSigned(event)
			if status != http.StatusOK || decision != connector.DeliveryDuplicate {
				t.Fatalf("completion replay HTTP=%d decision=%s", status, decision)
			}
		}
	}
	// Terminal state must reject delayed signed facts from either provider.
	if status, _ := postSigned(signAs(signed(9, "JOINED", "CLIENT"), connectorID, 7, private)); status != http.StatusConflict {
		t.Fatalf("late rejoin resurrected completed consultation: HTTP %d", status)
	}
	if status, _ := postSigned(signAs(signed(9, "TECHNICAL_FAILURE", "SYSTEM"), fallbackID, 3, fallbackPrivate)); status != http.StatusConflict {
		t.Fatalf("late technical failure changed terminal consultation: HTTP %d", status)
	}
	// The next Checker is a separate connection, equivalent to restarting API
	// without any in-process consultation sessions.
	restarted, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	timeline, timelineErr := restarted.ReadConsultationTimeline(ctx, hold.BookingID, intent.ClientIdentityID)
	if timelineErr != nil || timeline == nil || timeline.BookingState != "COMPLETED" ||
		timeline.ConsultationState != "COMPLETED" || len(timeline.Facts) < 8 {
		t.Fatalf("restart lost immutable consultation history: timeline=%+v error=%v", timeline, timelineErr)
	}
	result, err := restarted.ReadConsultationResult(ctx, hold.BookingID, intent.ClientIdentityID)
	if err != nil || result == nil || result.State != "COMPLETED" || result.ProviderInstanceID != fallbackID || result.CompletionEvidenceRef != "provider-evidence/ENDED/SYSTEM" {
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
	restartedDemand, err := demand.NewConformanceServiceWithStores(nil, restarted, restarted)
	if err != nil {
		t.Fatalf("reopened canonical journey after API restart: %v", err)
	}
	secondAPI := httpapi.New(httpapi.Options{Demand: restartedDemand, ConsultationProvider: restarted, ConsultationJourney: restarted, ClientSessionKey: sessionKey})
	// The authenticated owner must recover the exact captured financial truth
	// after creating a *new* API instance, rather than trusting a booking flag.
	persistedOrder, err := restartedDemand.ReadOrderForBooking(hold.BookingID, intent.ClientIdentityID)
	if err != nil || persistedOrder == nil || persistedOrder.BookingID != hold.BookingID ||
		persistedOrder.OrderID != checkout.OrderID || persistedOrder.PaymentState != "CAPTURED" ||
		persistedOrder.BookingState != booking.StateCompleted || persistedOrder.RefundState != "" {
		t.Fatalf("restarted signed provider ENDED must preserve CAPTURED payment with COMPLETED booking: %#v err=%v", persistedOrder, err)
	}
	if outsider, err := restartedDemand.ReadOrderForBooking(hold.BookingID, "foreign-client"); !errors.Is(err, demand.ErrOrderNotFound) || outsider != nil {
		t.Fatalf("foreign account recovered signed payment chain: %#v err=%v", outsider, err)
	}
	// Full paid user journey remains available by direct booking ID after
	// PostgreSQL connection/API restart; no browser sessionStorage token needed.
	bookingRead := httptest.NewRequest(http.MethodGet, "/v1/bookings/"+hold.BookingID, nil)
	bookingRead.AddCookie(sessionCookie)
	bookingResponse := httptest.NewRecorder()
	secondAPI.ServeHTTP(bookingResponse, bookingRead)
	if bookingResponse.Code != http.StatusOK || bookingResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("restarted API cannot read canonical booking: HTTP=%d body=%s", bookingResponse.Code, bookingResponse.Body.String())
	}
	var persistedBooking demand.BookingStatus
	if err := json.Unmarshal(bookingResponse.Body.Bytes(), &persistedBooking); err != nil {
		t.Fatal(err)
	}
	if persistedBooking.BookingID != hold.BookingID || persistedBooking.State != booking.StateCompleted ||
		persistedBooking.StartsAt == nil || persistedBooking.EndsAt == nil {
		t.Fatalf("restarted booking projection lost confirmed economic journey: %#v", persistedBooking)
	}
	foreignBooking := httptest.NewRecorder()
	secondAPI.ServeHTTP(foreignBooking, httptest.NewRequest(http.MethodGet, "/v1/bookings/"+hold.BookingID, nil))
	if foreignBooking.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous restarted booking leaked: HTTP %d", foreignBooking.Code)
	}
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
	// Full timeline is also available from an entirely new HTTP API process
	// with the same signed owner session after PostgreSQL reconnection.
	timelineRequest := httptest.NewRequest(http.MethodGet, "/v1/consultations/"+hold.BookingID+"/timeline", nil)
	timelineRequest.AddCookie(sessionCookie)
	timelineResponse := httptest.NewRecorder()
	secondAPI.ServeHTTP(timelineResponse, timelineRequest)
	if timelineResponse.Code != http.StatusOK || timelineResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("post-restart owner consultation history unreadable: HTTP=%d %s", timelineResponse.Code, timelineResponse.Body.String())
	}
	if apiResult.State != "COMPLETED" || apiResult.ProviderInstanceID != fallbackID || apiResult.CompletionEvidenceRef != "provider-evidence/ENDED/SYSTEM" {
		t.Fatalf("wrong HTTP restart result: %#v", apiResult)
	}
	outsiderRead := httptest.NewRecorder()
	secondAPI.ServeHTTP(outsiderRead, httptest.NewRequest(http.MethodGet, "/v1/consultations/"+hold.BookingID+"/result", nil))
	if outsiderRead.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated read HTTP=%d body=%s", outsiderRead.Code, outsiderRead.Body.String())
	}
}
