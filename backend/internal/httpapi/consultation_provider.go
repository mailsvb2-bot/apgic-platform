package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type ConsultationProviderStore interface {
	ApplyConsultationProviderWebhook(context.Context, connector.WebhookEnvelope, connector.WebhookPublicKeyResolver) (connector.DeliveryDecision, error)
	ReadConsultationResult(context.Context, string, string) (*connector.ConsultationResult, error)
}

func registerConsultationProvider(mux *http.ServeMux, store ConsultationProviderStore, keys connector.WebhookPublicKeyResolver, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("POST /v1/consultation-provider-webhooks", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSULT_PROVIDER_NOT_CONFIGURED", "Подтверждение провайдера связи не настроено.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var event connector.WebhookEnvelope
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CONSULT_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		// Verify signature before any domain mutation. The store also verifies
		// it inside its atomic connector-delivery transaction.
		if err := connector.VerifyWebhook(event, keys); err != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CONSULT_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		decision, err := store.ApplyConsultationProviderWebhook(r.Context(), event, keys)
		if err != nil {
			writeDemandError(w, r, http.StatusConflict, "CONSULT_PROVIDER_EVENT_REJECTED", "Событие провайдера не принято; состояние консультации не изменено.", false, nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"delivery_decision": string(decision)})
	})
	mux.HandleFunc("GET /v1/consultations/{bookingID}/result", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSULT_PROVIDER_NOT_CONFIGURED", "Результат консультации недоступен.", false, nil)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		result, err := store.ReadConsultationResult(r.Context(), r.PathValue("bookingID"), identity)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSULT_RESULT_UNAVAILABLE", "Результат консультации временно недоступен.", true, nil)
			return
		}
		if result == nil {
			writeDemandError(w, r, http.StatusNotFound, "CONSULT_RESULT_NOT_FOUND", "Результат консультации не найден.", false, nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
	})
}
