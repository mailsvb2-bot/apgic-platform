package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

type productOwnershipStore interface {
	audit.Appender
	IdentityOwnedProductCommercialOwner(productID, identityID string) (string, bool, error)
	UpdateIdentityOwnedProductCommercialOwner(productID, identityID, newCommercialOwnerRef string) (string, error)
}

type productCommercialOwnerRequest struct {
	CommercialOwnerRef string `json:"commercial_owner_ref"`
}

type productCommercialOwnerResponse struct {
	ProductID          string `json:"product_id"`
	CommercialOwnerRef string `json:"commercial_owner_ref"`
}

func registerHighRiskProductOwnership(
	mux *http.ServeMux,
	store productOwnershipStore,
	sessions *clientSessionManager,
	stepUp *stepUpManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("PATCH /v1/products/{productID}/commercial-owner", func(w http.ResponseWriter, r *http.Request) {
		if store == nil || sessions == nil || sessionConfigErr != nil || stepUp == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Защищённая операция временно недоступна.", true, nil)
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		productID := strings.TrimSpace(r.PathValue("productID"))
		currentOwner, found, err := store.IdentityOwnedProductCommercialOwner(productID, identityID)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Защищённая операция временно недоступна.", true, nil)
			return
		}
		if !found {
			writeDemandError(w, r, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Продукт не найден.", false, nil)
			return
		}

		var body productCommercialOwnerRequest
		if err := readJSONBody(r, &body); err != nil || strings.TrimSpace(body.CommercialOwnerRef) == "" {
			writeDemandError(w, r, http.StatusBadRequest, "PRODUCT_COMMERCIAL_OWNER_INVALID", "Укажите нового коммерческого владельца.", false, nil)
			return
		}
		body.CommercialOwnerRef = strings.TrimSpace(body.CommercialOwnerRef)

		auditID, err := persistentid.New()
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_AUDIT_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
			return
		}
		correlation := strings.TrimSpace(r.Header.Get("X-Correlation-Id"))
		if correlation == "" {
			correlation = "authz:" + auditID
		}
		result, err := (authz.Evaluator{
			PolicyVersion: "authz-policy-v1",
			Appender:      store,
		}).Authorize(authz.Input{
			Principal: authz.Principal{
				ID:       identityID,
				TenantID: identityID,
				Permissions: map[string]struct{}{
					"product.change_commercial_owner": {},
				},
				StepUpAt: stepUp.issuedAtFromRequest(r, identityID),
			},
			Resource:      authz.ResourceRef{ID: productID, TenantID: identityID},
			Action:        "product.change_commercial_owner",
			Risk:          authz.RiskHigh,
			Now:           now().UTC(),
			MaxStepUpAge:  stepUpTTL,
			CorrelationID: correlation,
			AuditRecordID: auditID,
		})
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_AUDIT_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
			return
		}
		if result.Decision == authz.StepUpRequired {
			writeDemandError(w, r, http.StatusPreconditionRequired, result.ReasonCode, "Для этого действия требуется повторное подтверждение личности.", false, []string{result.ReasonCode})
			return
		}
		if result.Decision != authz.Allow {
			writeDemandError(w, r, http.StatusForbidden, result.ReasonCode, "Доступ к операции запрещён.", false, []string{result.ReasonCode})
			return
		}

		_, err = store.UpdateIdentityOwnedProductCommercialOwner(productID, identityID, body.CommercialOwnerRef)
		if err != nil {
			writeDemandError(w, r, http.StatusConflict, "PRODUCT_OWNERSHIP_UPDATE_FAILED", "Не удалось изменить коммерческого владельца.", false, nil)
			return
		}
		_ = currentOwner
		writeJSON(w, http.StatusOK, productCommercialOwnerResponse{
			ProductID:          productID,
			CommercialOwnerRef: body.CommercialOwnerRef,
		})
	})
}
