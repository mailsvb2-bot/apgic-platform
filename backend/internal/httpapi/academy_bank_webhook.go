package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// The provider notification is not a learner request. No client cookie,
// browser identity or caller-provided buyer/price may authorize a capture.
// The canonical database writer validates the signed webhook against the
// persisted provider attempt and a fresh independent GetState response.
type academyBankCommitter interface {
	Commit(context.Context, string, []byte, *payments.TBankAcquiring) error
}

type academyBankCommitAdapter struct {
	call func(context.Context, string, []byte, *payments.TBankAcquiring) error
}

func (a academyBankCommitAdapter) Commit(ctx context.Context, id string, body []byte, bank *payments.TBankAcquiring) error {
	return a.call(ctx, id, body, bank)
}

func registerAcademyBankNotificationFromStore(mux *http.ServeMux, store academyStore) {
	if writer, ok := store.(interface {
		AcademyBankCommitNotification(context.Context, string, []byte, *payments.TBankAcquiring) error
	}); ok {
		registerAcademyBankWebhook(mux, academyBankCommitAdapter{call: writer.AcademyBankCommitNotification})
		return
	}
	registerAcademyBankWebhook(mux, nil)
}

// registerAcademyBankWebhook is disabled unless an operator explicitly
// configures the bank terminal, scoped trust roots, and release gate.
// It never uses secrets from requests. No generic HTTP handler can mint rights.
func registerAcademyBankWebhook(mux *http.ServeMux, committer academyBankCommitter) {
	mux.HandleFunc("POST /v1/payments/tbank/notification", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if os.Getenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED") != "true" || committer == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_WEBHOOK_DISABLED",
				"Банковское подтверждение не настроено.", true, nil)
			return
		}
		terminal := os.Getenv("TBANK_TERMINAL_KEY")
		secret := os.Getenv("TBANK_TERMINAL_PASSWORD")
		caPath := os.Getenv("APGIC_TBANK_CA_BUNDLE_PATH")
		if terminal == "" || secret == "" || caPath == "" || !strings.HasPrefix(caPath, "/") {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_CONFIG_UNAVAILABLE",
				"Банковское подтверждение не настроено.", true, nil)
			return
		}
		trusted, err := os.ReadFile(caPath)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_TRUST_UNAVAILABLE",
				"Проверка банка недоступна.", true, nil)
			return
		}
		bank, err := payments.NewTBankAcquiringWithCABundle(terminal, secret, trusted)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_TRUST_UNAVAILABLE",
				"Проверка банка недоступна.", true, nil)
			return
		}
		mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "application/json" {
			writeDemandError(w, r, http.StatusUnsupportedMediaType, "ACADEMY_BANK_INVALID_NOTIFICATION",
				"Некорректное банковское уведомление.", false, nil)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if err != nil || len(raw) == 0 {
			writeDemandError(w, r, http.StatusBadRequest, "ACADEMY_BANK_INVALID_NOTIFICATION",
				"Некорректное банковское уведомление.", false, nil)
			return
		}
		var input struct {
			OrderID string `json:"OrderId"`
		}
		if err = json.Unmarshal(raw, &input); err != nil || !academyUUID(input.OrderID) {
			writeDemandError(w, r, http.StatusBadRequest, "ACADEMY_BANK_INVALID_NOTIFICATION",
				"Некорректное банковское уведомление.", false, nil)
			return
		}
		// No entitlement on partial/forged status. The database writer checks the
		// signature, identity, amount, terminal, provider evidence and ledger.
		if err = committer.Commit(r.Context(), input.OrderID, raw, bank); err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, academy.ErrDenied) || errors.Is(err, academy.ErrInvalid) {
				status = http.StatusForbidden
			}
			writeDemandError(w, r, status, "ACADEMY_BANK_CAPTURE_NOT_VERIFIED",
				"Подтверждение банковской операции не принято.", status >= 500, nil)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}
