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
		adapter := academyBankCommitAdapter{call: writer.AcademyBankCommitNotification}
		if resolver, ok := store.(academyBankTerminalResolver); ok {
			registerAcademyBankWebhookWithResolver(mux, adapter, resolver)
		} else {
			registerAcademyBankWebhook(mux, adapter)
		}
		return
	}
	registerAcademyBankWebhook(mux, nil)
}

// A TerminalKey is an untrusted lookup key until the canonical bank token and
// independent GetState have been verified by the Commerce capture writer.
type academyBankTerminalResolver interface {
 AcademyBankForTerminal(context.Context,string)(*payments.TBankAcquiring,error)
}

// registerAcademyBankWebhook is disabled unless an operator explicitly
// configures the bank terminal, scoped trust roots, and release gate.
// It never uses secrets from requests. No generic HTTP handler can mint rights.
func registerAcademyBankWebhook(mux *http.ServeMux, committer academyBankCommitter) {
 registerAcademyBankWebhookWithResolver(mux,committer,nil)
}
func registerAcademyBankWebhookWithResolver(
 mux *http.ServeMux, committer academyBankCommitter,
 resolver academyBankTerminalResolver,
) {
	mux.HandleFunc("POST /v1/payments/tbank/notification", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if os.Getenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED") != "true" || committer == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_WEBHOOK_DISABLED",
				"Банковское подтверждение не настроено.", true, nil)
			return
		}
		var bank *payments.TBankAcquiring
		var err error
		if resolver == nil {
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
			bank, err = payments.NewTBankAcquiringWithCABundle(terminal, secret, trusted)
			if err != nil {
				writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_BANK_TRUST_UNAVAILABLE",
					"Проверка банка недоступна.", true, nil)
				return
			}
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
			TerminalKey string `json:"TerminalKey"`
		}
		if err = json.Unmarshal(raw, &input); err != nil || !academyUUID(input.OrderID) {
			writeDemandError(w, r, http.StatusBadRequest, "ACADEMY_BANK_INVALID_NOTIFICATION",
				"Некорректное банковское уведомление.", false, nil)
			return
		}
		if resolver != nil {
			if input.TerminalKey == "" || len(input.TerminalKey) > 128 ||
				strings.TrimSpace(input.TerminalKey) != input.TerminalKey {
				writeDemandError(w, r, http.StatusBadRequest,
					"ACADEMY_BANK_INVALID_NOTIFICATION",
					"Некорректное банковское уведомление.", false, nil)
				return
			}
			bank, err = resolver.AcademyBankForTerminal(r.Context(), input.TerminalKey)
			if err != nil || bank == nil {
				writeDemandError(w, r, http.StatusServiceUnavailable,
					"ACADEMY_BANK_CONFIG_UNAVAILABLE",
					"Банковское подтверждение не настроено.", true, nil)
				return
			}
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
