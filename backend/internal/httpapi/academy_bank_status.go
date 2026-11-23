package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Signed buyer readback from canonical persisted Commerce, never from a
// redirect query, payment cookie, or unsigned provider status parameter.
func registerAcademyBankOrderStatus(mux *http.ServeMux, store academyStore,
	sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("GET /v1/academy/bank/orders/{orderID}", func(w http.ResponseWriter, r *http.Request) {
		buyer, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		orderID := r.PathValue("orderID")
		if !academyUUID(orderID) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		reader, ok := store.(interface {
			AcademyReadBankOrderStatus(string, string) (academy.BankOrderStatus, error)
		})
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		status, err := reader.AcademyReadBankOrderStatus(buyer, orderID)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, status)
	})
}
