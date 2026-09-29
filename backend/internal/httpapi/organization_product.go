package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
)

type organizationProductStore interface {
	CreateOrganizationProduct(identityID, organizationID string, input commerce.OrganizationProductDraft) (commerce.ProductSnapshot, error)
	OrganizationProducts(identityID, organizationID string) ([]commerce.ProductSnapshot, error)
	OrganizationProduct(identityID, organizationID, productID string) (commerce.ProductSnapshot, error)
	PublishOrganizationProduct(identityID, organizationID, productID string, now time.Time) (commerce.ProductSnapshot, error)
}

type createOrganizationProductRequest struct {
	Name                  string   `json:"name"`
	DirectionID           string   `json:"direction_id"`
	CommercialOwnerRef    string   `json:"commercial_owner_ref"`
	AuthorRefs            []string `json:"author_refs"`
	RevenueBeneficiaryRef string   `json:"revenue_beneficiary_ref"`
}

func registerOrganizationProducts(
	mux *http.ServeMux,
	store organizationProductStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("GET /v1/organizations/{organizationID}/products", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeProductRuntimeError(w, r, http.StatusServiceUnavailable, "PRODUCT_STORE_UNAVAILABLE", "Контур продуктов временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		products, err := store.OrganizationProducts(identityID, strings.TrimSpace(r.PathValue("organizationID")))
		if err != nil {
			writeProductRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"products": products})
	})

	mux.HandleFunc("POST /v1/organizations/{organizationID}/products", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeProductRuntimeError(w, r, http.StatusServiceUnavailable, "PRODUCT_STORE_UNAVAILABLE", "Контур продуктов временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var body createOrganizationProductRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeProductRuntimeError(w, r, http.StatusBadRequest, "PRODUCT_INVALID", "Продукт не удалось прочитать.")
			return
		}
		product, err := store.CreateOrganizationProduct(
			identityID,
			strings.TrimSpace(r.PathValue("organizationID")),
			commerce.OrganizationProductDraft{
				Name:                  body.Name,
				DirectionID:           body.DirectionID,
				CommercialOwnerRef:    body.CommercialOwnerRef,
				AuthorRefs:            body.AuthorRefs,
				RevenueBeneficiaryRef: body.RevenueBeneficiaryRef,
			},
		)
		if err != nil {
			writeProductRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, product)
	})

	mux.HandleFunc("POST /v1/organizations/{organizationID}/products/{productID}/publish", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeProductRuntimeError(w, r, http.StatusServiceUnavailable, "PRODUCT_STORE_UNAVAILABLE", "Контур продуктов временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		product, err := store.PublishOrganizationProduct(
			identityID,
			strings.TrimSpace(r.PathValue("organizationID")),
			strings.TrimSpace(r.PathValue("productID")),
			now(),
		)
		if err != nil {
			writeProductRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, product)
	})
}

func writeProductRuntimeFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, commerce.ErrProductInvalid):
		writeProductRuntimeError(w, r, http.StatusBadRequest, "PRODUCT_INVALID", "Укажите продукт, направление и все явные роли.")
	case errors.Is(err, commerce.ErrProductDirectionInvalid):
		writeProductRuntimeError(w, r, http.StatusConflict, "PRODUCT_DIRECTION_INVALID", "Продукт можно создать и опубликовать только в активном направлении этой организации.")
	case errors.Is(err, commerce.ErrProductOwnerRequired):
		writeProductRuntimeError(w, r, http.StatusForbidden, "ORGANIZATION_OWNER_REQUIRED", "Для управления продуктами нужен активный владелец организации.")
	case errors.Is(err, commerce.ErrProductNotFound):
		writeProductRuntimeError(w, r, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Продукт не найден.")
	case errors.Is(err, commerce.ErrProductAlreadyPublished):
		writeProductRuntimeError(w, r, http.StatusConflict, "PRODUCT_ALREADY_PUBLISHED", "Продукт уже опубликован.")
	default:
		writeProductRuntimeError(w, r, http.StatusInternalServerError, "PRODUCT_INTERNAL_ERROR", "Контур продуктов не удалось обновить.")
	}
}

func writeProductRuntimeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeDemandError(w, r, status, code, message, status >= http.StatusInternalServerError, nil)
}
