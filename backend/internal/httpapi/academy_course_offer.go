package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The buyer only sends course identity and an idempotency key. Seller
// policy and PSP identity must be resolved from trusted Commerce state.
type academyBankCheckoutStarter interface {
	AcademyStartBankCheckout(context.Context, string, string, string) (academyBuyerCheckoutRedirect, error)
}
type academyBuyerCheckoutRedirect struct {
	OrderID    string `json:"order_id"`
	PaymentURL string `json:"payment_url"`
	State      string `json:"state"`
}

var academyCheckoutKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{8,128}$`)

// Inspect every raw spelling of Idempotency-Key; Header.Get hides
// duplicates injected through noncanonical casing. No two PSP Init calls
// may be justified by an ambiguous buyer key.
func academyBuyerCheckoutKey(headers http.Header) (string,bool) {
 var key string
 found:=false
 for name,values:=range headers {
  if !strings.EqualFold(name,"Idempotency-Key"){continue}
  if found||len(values)!=1{return "",false}
  found=true
  key=values[0]
 }
 return key,found&&academyCheckoutKeyPattern.MatchString(key)
}
func academyTrustedBankRedirect(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Port() != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "tbank.ru" || strings.HasSuffix(host, ".tbank.ru") ||
		host == "tinkoff.ru" || strings.HasSuffix(host, ".tinkoff.ru")
}

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
		buyer, ok := academyIdentity(w, r, sessions, sessionErr)
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
			if errors.Is(err, academy.ErrState) {
				writeDemandError(w, r, http.StatusConflict, "ACADEMY_OFFER_NOT_READY",
					"Покупка пока недоступна. Проверьте, что товар курса опубликован, цена сохранена и видеодоставка готова.",
					false, nil)
				return
			}
			academyHTTPError(w, r, err)
			return
		}
		// Browser cannot select seller/terminal/amount/fiscal receipt.
		key, validKey := academyBuyerCheckoutKey(r.Header)
		if !validKey {
			writeDemandError(w, r, http.StatusBadRequest,
				"ACADEMY_CHECKOUT_IDEMPOTENCY_REQUIRED",
				"Для оформления покупки нужен корректный ключ повторного запроса.", false, nil)
			return
		}
		// Only an empty JSON object is valid. Browser pricing, merchant IDs,
		// tax options and terminal credentials never enter a signed checkout.
		var input struct{}
		if !academyRead(w, r, &input) { return }
		starter, ready := store.(academyBankCheckoutStarter)
		if !ready {
			writeDemandError(w, r, http.StatusServiceUnavailable,
				"ACADEMY_EXTERNAL_CHECKOUT_NOT_CONFIGURED",
				"Оплата через Т-Банк пока не подключена. APGIC не принимает деньги и не выдаёт доступ без подтверждения банка.",
				true, nil)
			return
		}
		redirect, err := starter.AcademyStartBankCheckout(r.Context(), buyer, r.PathValue("courseID"), key)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		if !academyUUID(redirect.OrderID) || redirect.State != "SENT" ||
			!academyTrustedBankRedirect(redirect.PaymentURL) {
			writeDemandError(w, r, http.StatusServiceUnavailable,
				"ACADEMY_BANK_REDIRECT_INVALID",
				"Платёжная форма недоступна. Оплата не подтверждена.", true, nil)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, redirect)
	})
}
