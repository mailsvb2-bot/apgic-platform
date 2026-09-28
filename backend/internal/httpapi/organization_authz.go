package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const organizationAuthzPolicyVersion = "authz-policy-v1"

type organizationAuthorizationStore interface {
	audit.Appender
	ActiveOrganizationMembership(identityID, organizationID string) (bool, error)
	OrganizationPrivateName(organizationID string) (string, bool, error)
}

type privateOrganizationProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func registerOrganizationAuthorization(
	mux *http.ServeMux,
	store organizationAuthorizationStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("GET /v1/organizations/{organizationID}/private-profile", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Авторизация организации временно недоступна.", true, nil)
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		contextID := strings.TrimSpace(r.Header.Get("X-Organization-Context"))
		if contextID == "" {
			writeDemandError(w, r, http.StatusForbidden, "AUTH_TENANT_CONTEXT_REQUIRED", "Организационный контекст не подтверждён.", false, nil)
			return
		}
		member, err := store.ActiveOrganizationMembership(identityID, contextID)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Авторизация организации временно недоступна.", true, nil)
			return
		}
		if !member {
			writeDemandError(w, r, http.StatusForbidden, "AUTH_TENANT_CONTEXT_DENIED", "Организационный контекст не подтверждён.", false, nil)
			return
		}

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
			PolicyVersion: organizationAuthzPolicyVersion,
			Appender:      store,
		}).Authorize(authz.Input{
			Principal: authz.Principal{
				ID:       identityID,
				TenantID: contextID,
				Permissions: map[string]struct{}{
					"organization.read_private": {},
				},
			},
			Resource: authz.ResourceRef{
				ID:       "private-profile",
				TenantID: strings.TrimSpace(r.PathValue("organizationID")),
			},
			Action:        "organization.read_private",
			Risk:          authz.RiskNormal,
			Now:           now().UTC(),
			CorrelationID: correlation,
			AuditRecordID: auditID,
		})
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_AUDIT_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
			return
		}
		if result.Decision != authz.Allow {
			writeDemandError(w, r, http.StatusForbidden, result.ReasonCode, "Доступ к ресурсу запрещён.", false, []string{result.ReasonCode})
			return
		}

		organizationID := strings.TrimSpace(r.PathValue("organizationID"))
		name, found, err := store.OrganizationPrivateName(organizationID)
		switch {
		case err != nil:
			writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Организация временно недоступна.", true, nil)
		case !found:
			writeDemandError(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Организация не найдена.", false, nil)
		default:
			writeJSON(w, http.StatusOK, privateOrganizationProfile{ID: organizationID, Name: name})
		}
	})
}
