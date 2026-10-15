package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
)

// FinanceStore is the R4 provider-executed financial runtime. APGIC only
// records provider evidence and exposes owner-scoped status; it never moves
// or holds funds.
type FinanceStore interface {
	ApplyFinanceProviderWebhook(context.Context, connector.WebhookEnvelope, connector.WebhookPublicKeyResolver) (finance.EvidenceDecision, error)
	ReadFinanceOrderStatus(context.Context, string, string) (*finance.OrderStatus, error)
}

func registerFinance(mux *http.ServeMux, store FinanceStore, keys connector.WebhookPublicKeyResolver, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("POST /v1/finance-provider-webhooks", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_PROVIDER_NOT_CONFIGURED", "Финансовые события провайдера не настроены.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var event connector.WebhookEnvelope
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "FINANCE_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		// Verify before any mutation; the store verifies again atomically.
		if err := connector.VerifyWebhook(event, keys); err != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "FINANCE_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		decision, err := store.ApplyFinanceProviderWebhook(r.Context(), event, keys)
		if err != nil {
			writeDemandError(w, r, http.StatusConflict, "FINANCE_PROVIDER_EVENT_REJECTED", "Финансовое событие провайдера не принято; история не изменена.", false, nil)
			return
		}
		writeJSON(w, http.StatusOK, decision)
	})
	mux.HandleFunc("GET /v1/finance/orders/{orderID}/status", func(w http.ResponseWriter, r *http.Request) {
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Сессия клиента недоступна.", false, nil)
			return
		}
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_STATUS_NOT_CONFIGURED", "Финансовый статус недоступен.", false, nil)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		status, err := store.ReadFinanceOrderStatus(r.Context(), r.PathValue("orderID"), identity)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_STATUS_UNAVAILABLE", "Финансовый статус временно недоступен.", true, nil)
			return
		}
		if status == nil {
			writeDemandError(w, r, http.StatusNotFound, "FINANCE_STATUS_NOT_FOUND", "Заказ не найден.", false, nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, status)
	})
}
