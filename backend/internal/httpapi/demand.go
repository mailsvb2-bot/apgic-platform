package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

type errorEnvelope struct {
	Code              string   `json:"code"`
	MessageSafe       string   `json:"message_safe"`
	CorrelationID     string   `json:"correlation_id"`
	Retryable         bool     `json:"retryable"`
	PolicyReasonCodes []string `json:"policy_reason_codes,omitempty"`
}

type createIntentRequest struct {
	FreeText string `json:"free_text"`
}

type confirmIntentRequest struct {
	Topics  []string          `json:"topics"`
	Goals   []string          `json:"goals"`
	Context map[string]string `json:"context"`
}

type holdRequest struct {
	HelpIntentID     string `json:"help_intent_id"`
	SlotID           string `json:"slot_id"`
	ClientIdentityID string `json:"client_identity_id"`
}

type checkoutRequest struct {
	HoldID           string `json:"hold_id"`
	ClientIdentityID string `json:"client_identity_id"`
	MethodCode       string `json:"method_code"`
}

type providerEventRequest struct {
	ProviderID      string `json:"provider_id"`
	ProviderEventID string `json:"provider_event_id"`
	OrderID         string `json:"order_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	Outcome         string `json:"outcome"`
}

type cancellationRequest struct {
	OrderID    string `json:"order_id"`
	ReasonCode string `json:"reason_code"`
}

func registerDemand(mux *http.ServeMux, service *demand.Service) {
	mux.HandleFunc("POST /v1/help-intents", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body createIntentRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "HELP_INTENT_INVALID", "Запрос не удалось прочитать.", false, nil)
			return
		}
		intent, err := service.CreateIntent(body.FreeText)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, intent)
	})

	mux.HandleFunc("POST /v1/help-intents/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body confirmIntentRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "HELP_INTENT_INVALID", "Подтверждение не удалось прочитать.", false, nil)
			return
		}
		intent, err := service.ConfirmIntent(r.PathValue("id"), body.Topics, body.Goals, body.Context)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, intent)
	})

	mux.HandleFunc("GET /v1/search", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		view, err := service.Search(r.URL.Query().Get("topic"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("POST /v1/search/rebuild", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			Topic string `json:"topic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "SEARCH_INVALID", "Запрос поиска не удалось прочитать.", false, nil)
			return
		}
		view, err := service.RebuildSearch(body.Topic)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("POST /v1/search/stale", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			Topic        string `json:"topic"`
			SpecialistID string `json:"specialist_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "SEARCH_INVALID", "Запрос поиска не удалось прочитать.", false, nil)
			return
		}
		view, err := service.MarkSearchStale(body.Topic, body.SpecialistID)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("GET /v1/help-intents/{id}/matches", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		cards, topic, err := service.Matches(r.PathValue("id"), r.URL.Query().Get("topic"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"topic":                  topic,
			"ranking_policy_version": demand.RankingVersion,
			"catalog_mode":           demand.CatalogModeConformance,
			"matches":                cards,
		})
	})

	mux.HandleFunc("GET /v1/specialists/{id}/slots", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		slots, err := service.Slots(r.PathValue("id"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"specialist_id": r.PathValue("id"),
			"slots":         slots,
		})
	})

	mux.HandleFunc("POST /v1/slot-holds", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body holdRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "BOOK_HOLD_INVALID", "Запрос удержания не удалось прочитать.", false, nil)
			return
		}
		hold, err := service.AcquireHold(body.HelpIntentID, body.SlotID, body.ClientIdentityID)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, hold)
	})

	mux.HandleFunc("GET /v1/slot-holds/{id}/checkout-options", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		options, err := service.CheckoutOptions(r.PathValue("id"), r.URL.Query().Get("client_identity_id"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hold_id":             r.PathValue("id"),
			"apgic_accepts_funds": false,
			"options":             options,
		})
	})

	mux.HandleFunc("POST /v1/checkout-instructions", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body checkoutRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CHECKOUT_INVALID", "Поручение на оплату не удалось прочитать.", false, nil)
			return
		}
		instruction, err := service.CreateCheckout(body.HoldID, body.ClientIdentityID, body.MethodCode)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, instruction)
	})

	mux.HandleFunc("POST /v1/provider-events", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body providerEventRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "EVIDENCE_INVALID", "Сообщение провайдера не удалось прочитать.", false, nil)
			return
		}
		evidence, err := service.ApplyProviderEvent(demand.ProviderEvent{
			ProviderID:      body.ProviderID,
			ProviderEventID: body.ProviderEventID,
			OrderID:         body.OrderID,
			AmountMinor:     body.AmountMinor,
			Currency:        body.Currency,
			Outcome:         body.Outcome,
		})
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if evidence.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, evidence)
	})

	mux.HandleFunc("GET /v1/bookings/{id}/fulfillment", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		notice, join, err := service.Fulfillment(r.PathValue("id"), r.URL.Query().Get("identity_id"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"notice": notice, "join": join})
	})

	mux.HandleFunc("POST /v1/consultations/{bookingID}/presence", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		view, err := service.RecordSessionPresence(r.PathValue("bookingID"))
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if view.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, view)
	})

	mux.HandleFunc("POST /v1/consultations/{bookingID}/failures", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			Kind        string `json:"kind"`
			EvidenceRef string `json:"evidence_ref"`
			Recoverable bool   `json:"recoverable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "COMM_RECOVERY_INVALID", "Сообщение о сбое не удалось прочитать.", false, nil)
			return
		}
		view, err := service.ReportProviderFailure(r.PathValue("bookingID"), body.Kind, body.EvidenceRef, body.Recoverable)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if view.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, view)
	})

	mux.HandleFunc("POST /v1/consultations/{bookingID}/recovery", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			EvidenceRef string `json:"evidence_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "COMM_RECOVERY_INVALID", "Сообщение о восстановлении не удалось прочитать.", false, nil)
			return
		}
		view, err := service.SucceedRecovery(r.PathValue("bookingID"), body.EvidenceRef)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if view.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, view)
	})

	mux.HandleFunc("POST /v1/consultations/{bookingID}/growth-export", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			PurposeConsent bool `json:"purpose_consent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "GROWTH_EXPORT_INVALID", "Запрос выгрузки не удалось прочитать.", false, nil)
			return
		}
		exported, err := service.ExportSessionToGrowth(r.PathValue("bookingID"), body.PurposeConsent)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, exported)
	})

	mux.HandleFunc("POST /v1/consultations/{bookingID}/complete", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			EvidenceRef string `json:"evidence_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CONSULT_INVALID", "Завершение консультации не удалось прочитать.", false, nil)
			return
		}
		view, err := service.CompleteSession(r.PathValue("bookingID"), body.EvidenceRef)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if view.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, view)
	})

	mux.HandleFunc("POST /v1/account-deletions", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body struct {
			IdentityID string `json:"identity_id"`
			Source     string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "ACCOUNT_DELETION_INVALID", "Запрос удаления не удалось прочитать.", false, nil)
			return
		}
		deletion, err := service.DeleteAccount(body.IdentityID, body.Source)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if deletion.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, deletion)
	})

	mux.HandleFunc("POST /v1/cancellations", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEMAND_CATALOG_UNAVAILABLE", "Каталог спроса не подключён.", false, nil)
			return
		}
		var body cancellationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CANCEL_INVALID", "Запрос отмены не удалось прочитать.", false, nil)
			return
		}
		cancellation, err := service.CancelOrder(body.OrderID, body.ReasonCode)
		if err != nil {
			writeDemandFailure(w, r, err)
			return
		}
		status := http.StatusCreated
		if cancellation.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, cancellation)
	})
}

func writeDemandFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, demand.ErrTextRequired):
		writeDemandError(w, r, http.StatusBadRequest, "HELP_INTENT_TEXT_REQUIRED", "Опишите запрос своими словами.", false, nil)
	case errors.Is(err, demand.ErrTopicRequired):
		writeDemandError(w, r, http.StatusBadRequest, "HELP_INTENT_TOPIC_REQUIRED", "Выберите хотя бы одну тему.", false, nil)
	case errors.Is(err, demand.ErrTopicUnknown):
		writeDemandError(w, r, http.StatusBadRequest, "HELP_INTENT_TOPIC_UNKNOWN", "Этой темы нет в каталоге.", false, nil)
	case errors.Is(err, demand.ErrIntentNotFound), errors.Is(err, demand.ErrSpecialistNotFound), errors.Is(err, demand.ErrSlotNotFound):
		writeDemandError(w, r, http.StatusNotFound, "NOT_FOUND", "Запись не найдена.", false, nil)
	case errors.Is(err, demand.ErrNotConfirmed):
		writeDemandError(w, r, http.StatusConflict, "HELP_INTENT_NOT_CONFIRMED", "Сначала подтвердите, как мы поняли запрос.", false, []string{"HELP_INTENT_NOT_CONFIRMED"})
	case errors.Is(err, demand.ErrIdentityMismatch):
		writeDemandError(w, r, http.StatusForbidden, "HELP_INTENT_IDENTITY_MISMATCH", "Этот запрос принадлежит другому клиенту.", false, nil)
	case errors.Is(err, demand.ErrSlotHeld):
		writeDemandError(w, r, http.StatusConflict, "BOOK_SLOT_HELD", "Этот слот уже удерживается другим клиентом.", false, []string{"BOOK_SLOT_HELD"})
	case errors.Is(err, demand.ErrSlotUnavailable), errors.Is(err, demand.ErrSlotNotExclusive):
		writeDemandError(w, r, http.StatusConflict, "BOOK_SLOT_NOT_AVAILABLE", "Слот нельзя удержать.", false, []string{"BOOK_SLOT_NOT_AVAILABLE"})
	case errors.Is(err, demand.ErrHoldNotFound):
		writeDemandError(w, r, http.StatusNotFound, "NOT_FOUND", "Удержание не найдено.", false, nil)
	case errors.Is(err, demand.ErrHoldNotActive):
		writeDemandError(w, r, http.StatusConflict, "BOOK_HOLD_NOT_ACTIVE", "Удержание слота уже не активно.", false, nil)
	case errors.Is(err, demand.ErrMethodNotEligible):
		writeDemandError(w, r, http.StatusConflict, "PAY_METHOD_UNSUPPORTED", "Этот способ оплаты недоступен у внешнего провайдера.", false, []string{"PAY_METHOD_UNSUPPORTED"})
	case errors.Is(err, demand.ErrCheckoutLocked):
		writeDemandError(w, r, http.StatusConflict, "CHECKOUT_METHOD_LOCKED", "Способ оплаты уже зафиксирован.", false, nil)
	case errors.Is(err, demand.ErrCustodyForbidden):
		writeDemandError(w, r, http.StatusConflict, "PAY_CUSTODY_FORBIDDEN", "APGIC не принимает деньги.", false, []string{"PAY_EXTERNAL_EXECUTION_REQUIRED"})
	case errors.Is(err, demand.ErrOrderNotFound):
		writeDemandError(w, r, http.StatusNotFound, "NOT_FOUND", "Поручение на оплату не найдено.", false, nil)
	case errors.Is(err, demand.ErrEvidenceMismatch):
		writeDemandError(w, r, http.StatusConflict, "PAY_EVIDENCE_MISMATCH", "Сообщение провайдера не совпадает с поручением.", false, nil)
	case errors.Is(err, demand.ErrDuplicateEffect):
		writeDemandError(w, r, http.StatusConflict, "PAY_DUPLICATE_EFFECT", "По этому заказу уже есть одно подтверждение провайдера.", false, nil)
	case errors.Is(err, demand.ErrCancelNotAllowed):
		writeDemandError(w, r, http.StatusConflict, "BOOK_CANCEL_DENIED", "Эту бронь нельзя отменить.", false, nil)
	case errors.Is(err, demand.ErrConsultNotReady):
		writeDemandError(w, r, http.StatusConflict, "CONSULT_NOT_READY", "Консультацию нельзя завершить, пока нет фактов входа обеих сторон.", false, nil)
	case errors.Is(err, demand.ErrConsultEvidence):
		writeDemandError(w, r, http.StatusConflict, "CONSULT_EVIDENCE_REQUIRED", "Завершение требует доказательство провайдера связи, а не таймер.", false, []string{"CONSULT_EVIDENCE_REQUIRED"})
	case errors.Is(err, demand.ErrRecoveryInvalid):
		writeDemandError(w, r, http.StatusBadRequest, "COMM_RECOVERY_INVALID", "Сбой связи не распознан.", false, nil)
	case errors.Is(err, demand.ErrPurposeConsent):
		writeDemandError(w, r, http.StatusConflict, "DATA_PURPOSE_CONSENT_REQUIRED", "Сырую запись консультации нельзя передать в рост без отдельного согласия.", false, []string{"DATA_PURPOSE_CONSENT_REQUIRED"})
	case errors.Is(err, demand.ErrNotDeletion):
		writeDemandError(w, r, http.StatusConflict, "ACCOUNT_DEACTIVATION_IS_NOT_DELETION", "Деактивация не считается удалением учётной записи.", false, []string{"ACCOUNT_DEACTIVATION_IS_NOT_DELETION"})
	default:
		writeDemandError(w, r, http.StatusBadRequest, "DEMAND_REJECTED", "Запрос отклонён.", false, nil)
	}
}

func writeDemandError(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool, reasons []string) {
	correlation := strings.TrimSpace(r.Header.Get("X-Correlation-Id"))
	if correlation == "" {
		correlation = "missing"
	}
	writeJSON(w, status, errorEnvelope{
		Code:              code,
		MessageSafe:       message,
		CorrelationID:     correlation,
		Retryable:         retryable,
		PolicyReasonCodes: reasons,
	})
}
