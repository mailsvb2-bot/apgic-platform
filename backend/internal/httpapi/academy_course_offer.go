package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCourseCheckoutQuoteStore interface {
	AcademyCheckoutOffer(courseID string) (academy.CourseOffer, error)
}

// Keep the HTTP layer a thin adapter to the existing Academy/Commerce owner.
// Pricing never grants enrollment or changes a product's settlement recipient.
type academyCourseOfferStore interface {
	AcademySetCourseOffer(actor, courseID, mode string, priceMinor int64) (academy.CourseOffer, error)
}

func registerAcademyCourseOffers(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/offer", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		writer, ok := store.(academyCourseOfferStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			AccessMode string `json:"access_mode"`
			PriceMinor int64  `json:"price_minor"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		offer, err := writer.AcademySetCourseOffer(actor, r.PathValue("courseID"), input.AccessMode, input.PriceMinor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, offer)
	})
	// The bank adapter is not present. Never create an imaginary order or
	// expose a fake redirect/payment success. The Commerce payment control
	// plane remains the ONLY future checkout/PSP verification owner.
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/checkout", func(w http.ResponseWriter, r *http.Request) {
		_, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		if !academyUUID(r.PathValue("courseID")) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		quote, available := store.(academyCourseCheckoutQuoteStore)
		if !available {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if _, err := quote.AcademyCheckoutOffer(r.PathValue("courseID")); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeDemandError(w, r, http.StatusServiceUnavailable,
			"ACADEMY_EXTERNAL_CHECKOUT_NOT_CONFIGURED",
			"Оплата через Т-Банк пока не подключена. APGIC не принимает деньги и не выдаёт доступ без подтверждения банка.",
			true, nil)
	})
}
