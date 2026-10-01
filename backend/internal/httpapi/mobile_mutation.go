package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const mobileCheckoutMutationOperation = "CREATE_CHECKOUT"

var mobileMutationKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const mobileCheckoutMutationOperation = "CREATE_CHECKOUT"

)
var mobileMutationCorrelationPattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,160}package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const mobileCheckoutMutationOperation = "CREATE_CHECKOUT"

)

type mobileCheckoutMutationRequest struct {
	HoldID     string `json:"hold_id"`
	MethodCode string `json:"method_code"`
}

type mobileCheckoutMutationResponse struct {
	ContractVersion string                      `json:"contract_version"`
	Outcome         string                      `json:"outcome"`
	ReasonCode      string                      `json:"reason_code"`
	SideEffectRef   string                      `json:"side_effect_ref"`
	Checkout        *demand.CheckoutInstruction `json:"checkout"`
}

func registerMobileCheckoutMutation(
	mux *http.ServeMux,
	service *demand.Service,
	store mutation.Store,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("POST /v1/mobile/checkout-instructions", func(w http.ResponseWriter, r *http.Request) {
		if service == nil || store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if !mobileMutationKeyPattern.MatchString(idempotencyKey) {
			writeDemandError(w, r, http.StatusBadRequest, "MUTATION_IDEMPOTENCY_KEY_INVALID", "Ключ повтора действия некорректен.", false, nil)
			return
		}
		correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-Id"))
		if !mobileMutationCorrelationPattern.MatchString(correlationID) {
			writeDemandError(w, r, http.StatusBadRequest, "MUTATION_CORRELATION_ID_INVALID", "Идентификатор корреляции действия некорректен.", false, nil)
			return
		}
		var body mobileCheckoutMutationRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CHECKOUT_INVALID", "Поручение на оплату не удалось прочитать.", false, nil)
			return
		}
		body.HoldID = strings.TrimSpace(body.HoldID)
		body.MethodCode = strings.TrimSpace(body.MethodCode)
		if body.HoldID == "" || body.MethodCode == "" {
			writeDemandError(w, r, http.StatusBadRequest, "CHECKOUT_INVALID", "Параметры поручения на оплату обязательны.", false, nil)
			return
		}

		digest := checkoutMutationDigest(body)
		mutationID, err := persistentid.New()
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
			return
		}
		claim, err := store.Claim(r.Context(), mutationID, mutation.Envelope{
			IdentityID:     identityID,
			Operation:      mobileCheckoutMutationOperation,
			IdempotencyKey: idempotencyKey,
			CorrelationID:  correlationID,
			RequestDigest:  digest,
		}, now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
			return
		}

		switch claim.Outcome {
		case mutation.OutcomeConflict:
			writeDemandError(w, r, http.StatusConflict, "MUTATION_IDEMPOTENCY_CONFLICT", "Этот ключ повтора уже связан с другим действием.", false, []string{"MUTATION_IDEMPOTENCY_CONFLICT"})
			return
		case mutation.OutcomeFailed:
			code := claim.FailureCode
			if code == "" {
				code = "MUTATION_PREVIOUSLY_FAILED"
			}
			writeDemandError(w, r, http.StatusConflict, code, "Предыдущее выполнение этого действия завершилось ошибкой.", false, []string{code})
			return
		case mutation.OutcomeClaimed, mutation.OutcomeDuplicate, mutation.OutcomeDuplicateApplied:
		default:
			writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
			return
		}

		instruction, err := service.CreateCheckout(body.HoldID, identityID, body.MethodCode)
		if err != nil {
			if failureCode, terminal := terminalCheckoutMutationFailure(err); terminal {
				if _, failErr := store.Fail(r.Context(), claim.MutationID, failureCode, now().UTC()); failErr != nil {
					writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
					return
				}
			}
			writeDemandFailure(w, r, err)
			return
		}

		sideEffectRef := "checkout/" + instruction.ID
		if claim.Outcome == mutation.OutcomeDuplicateApplied {
			if claim.SideEffectRef != sideEffectRef {
				writeDemandError(w, r, http.StatusConflict, "MUTATION_SIDE_EFFECT_CONFLICT", "Повтор действия не совпал с подтверждённым серверным результатом.", false, []string{"MUTATION_SIDE_EFFECT_CONFLICT"})
				return
			}
			writeJSON(w, http.StatusOK, mobileCheckoutMutationResponse{
				ContractVersion: "offline-checkout-mutation-v1",
				Outcome:         "DUPLICATE_APPLIED",
				ReasonCode:      "MUTATION_DUPLICATE_APPLIED",
				SideEffectRef:   sideEffectRef,
				Checkout:        instruction,
			})
			return
		}

		finalized, err := store.Finalize(r.Context(), claim.MutationID, sideEffectRef, now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MUTATION_RUNTIME_UNAVAILABLE", "Синхронизация действия временно недоступна.", true, nil)
			return
		}
		if !finalized.Changed && finalized.ReasonCode != "MUTATION_ALREADY_APPLIED" {
			writeDemandError(w, r, http.StatusConflict, finalized.ReasonCode, "Серверный результат действия не удалось подтвердить.", false, []string{finalized.ReasonCode})
			return
		}

		status := http.StatusCreated
		outcome := "APPLIED"
		reason := "MUTATION_APPLIED"
		if claim.Outcome == mutation.OutcomeDuplicate || !finalized.Changed {
			status = http.StatusOK
			outcome = "DUPLICATE_APPLIED"
			reason = "MUTATION_DUPLICATE_APPLIED"
		}
		writeJSON(w, status, mobileCheckoutMutationResponse{
			ContractVersion: "offline-checkout-mutation-v1",
			Outcome:         outcome,
			ReasonCode:      reason,
			SideEffectRef:   sideEffectRef,
			Checkout:        instruction,
		})
	})
}

func checkoutMutationDigest(body mobileCheckoutMutationRequest) string {
	sum := sha256.Sum256([]byte("offline-checkout-v1\n" + body.HoldID + "\n" + body.MethodCode))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func terminalCheckoutMutationFailure(err error) (string, bool) {
	switch {
	case errors.Is(err, demand.ErrHoldNotFound):
		return "BOOK_HOLD_NOT_FOUND", true
	case errors.Is(err, demand.ErrIdentityMismatch):
		return "BOOK_HOLD_IDENTITY_MISMATCH", true
	case errors.Is(err, demand.ErrHoldNotActive):
		return "BOOK_HOLD_NOT_ACTIVE", true
	case errors.Is(err, demand.ErrSlotBooked):
		return "BOOK_SLOT_BOOKED", true
	case errors.Is(err, demand.ErrMethodNotEligible):
		return "PAY_METHOD_UNSUPPORTED", true
	case errors.Is(err, demand.ErrCheckoutLocked):
		return "CHECKOUT_METHOD_LOCKED", true
	case errors.Is(err, demand.ErrCustodyForbidden):
		return "PAY_CUSTODY_FORBIDDEN", true
	default:
		return "", false
	}
}
