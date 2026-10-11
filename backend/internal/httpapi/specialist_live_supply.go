package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// Authenticated specialist-owned priced offers. Only PostgreSQL verifies
// qualification, role, published Product and Organization ownership.
func registerSpecialistBookableOffers(
	mux *http.ServeMux, store specialist.Store,
	sessions *clientSessionManager, sessionConfigErr error,
) {
	supply, ok := store.(specialist.OfferStore)
	if !ok {
		return
	}

	mux.HandleFunc("POST /v1/specialist/offers", func(w http.ResponseWriter, r *http.Request) {
		identityID, valid := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !valid {
			return
		}
		var body struct {
			ProductID  string `json:"product_id"`
			TopicID    string `json:"topic_id"`
			PriceMinor int64  `json:"price_minor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_OFFER_INVALID", "Укажите существующий продукт, направление и цену.")
			return
		}
		offer, err := supply.CreateBookableOffer(identityID, body.ProductID, body.TopicID, body.PriceMinor)
		if err != nil {
			writeBookableSupplyFailure(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, offer)
	})
	mux.HandleFunc("GET /v1/specialist/offers", func(w http.ResponseWriter, r *http.Request) {
		identityID, valid := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !valid {
			return
		}
		offers, err := supply.ListBookableOffers(identityID)
		if err != nil {
			writeBookableSupplyFailure(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
	})
	mux.HandleFunc("POST /v1/specialist/offers/{id}/slots", func(w http.ResponseWriter, r *http.Request) {
		identityID, valid := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !valid {
			return
		}
		var body struct {
			StartsAt time.Time `json:"starts_at"`
			EndsAt   time.Time `json:"ends_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_SLOT_INVALID", "Укажите начало и окончание консультации.")
			return
		}
		slot, err := supply.AddBookableSlot(identityID, r.PathValue("id"), body.StartsAt, body.EndsAt)
		if err != nil {
			writeBookableSupplyFailure(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, slot)
	})
}

func writeBookableSupplyFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, specialist.ErrOfferInvalid):
		writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_OFFER_INVALID", "Проверьте опубликованный продукт, направление и положительную цену.")
	case errors.Is(err, specialist.ErrOfferNotAuthorized):
		writeSpecialistError(w, r, http.StatusForbidden, "SPECIALIST_OFFER_OWNER_REQUIRED", "Продукт или предложение не принадлежит вашему профессиональному кабинету.")
	case errors.Is(err, specialist.ErrOfferNotEligible):
		writeSpecialistError(w, r, http.StatusConflict, "SPECIALIST_OFFER_NOT_ELIGIBLE", "Требуются проверенные компетенции, действующая публикация и активный продукт.")
	case errors.Is(err, specialist.ErrSlotInvalid):
		writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_SLOT_INVALID", "Укажите будущее свободное время и длительность до четырёх часов.")
	case errors.Is(err, specialist.ErrSlotConflict):
		writeSpecialistError(w, r, http.StatusConflict, "SPECIALIST_SLOT_CONFLICT", "Это время уже занято или пересекается с другим слотом.")
	default:
		writeSpecialistFailure(w, r, err)
	}
}
