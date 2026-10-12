package httpapi

import (
	"errors"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

func registerPublicSpecialistOffers(mux *http.ServeMux, store specialist.Store) {
	offersStore, ok := store.(specialist.PublicBookableOfferStore)
	mux.HandleFunc("GET /v1/specialists/{id}/offers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !ok || offersStore == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_OFFERS_UNAVAILABLE", "Предложения пока недоступны.")
			return
		}
		id := r.PathValue("id")
		offers, err := offersStore.PublicOffers(r.Context(), id)
		if errors.Is(err, specialist.ErrPublicProfileNotFound) {
			writeSpecialistError(w, r, http.StatusNotFound, "SPECIALIST_NOT_PUBLIC", "Анкета не опубликована.")
			return
		}
		if err != nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_OFFERS_UNAVAILABLE", "Не удалось проверить доступные предложения.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"specialist_id":     id,
			"offers":            offers,
			"availability_rule": "CURRENT_ELIGIBLE_UNHELD_SLOTS",
		})
	})
}
