package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const (
	deepLinkTTL             = 15 * time.Minute
	maxDeepLinkIssueBodySize = 4 * 1024
)

type deepLinkResourceStore interface {
	DeepLinkResource(kind mobile.LinkKind, targetID string) (mobile.DeepLinkResource, bool, error)
}

type issueDeepLinkRequest struct {
	Kind     mobile.LinkKind `json:"kind"`
	TargetID string          `json:"target_id"`
}

type issueDeepLinkResponse struct {
	Token        string    `json:"token"`
	UniversalURL string    `json:"universal_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type deepLinkResolutionResponse struct {
	Decision             string `json:"decision"`
	ReasonCode           string `json:"reason_code"`
	CanonicalPath        string `json:"canonical_path,omitempty"`
	CanonicalWebFallback string `json:"canonical_web_fallback,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
}

func registerMobileDeepLinks(
	mux *http.ServeMux,
	store deepLinkResourceStore,
	tokens *mobile.DeepLinkTokenManager,
	tokenConfigErr error,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("POST /v1/mobile/deep-links", func(w http.ResponseWriter, r *http.Request) {
		if store == nil || tokens == nil || tokenConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEEPLINK_UNAVAILABLE", "Безопасные ссылки временно недоступны.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxDeepLinkIssueBodySize)
		var body issueDeepLinkRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, mobile.ReasonLinkInvalid, "Параметры ссылки некорректны.", false, nil)
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, mobile.ReasonLinkInvalid, "Параметры ссылки некорректны.", false, nil)
			return
		}
		body.TargetID = strings.TrimSpace(body.TargetID)
		resource, found, err := store.DeepLinkResource(body.Kind, body.TargetID)
		switch {
		case err != nil:
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEEPLINK_RESOURCE_UNAVAILABLE", "Ресурс ссылки временно недоступен.", true, nil)
			return
		case !found:
			writeDemandError(w, r, http.StatusNotFound, "DEEPLINK_RESOURCE_NOT_FOUND", "Ресурс не найден.", false, nil)
			return
		}
		if resource.AccessClass == mobile.LinkProtectedResource {
			identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
			if !ok {
				return
			}
			if identityID != resource.SubjectIdentityID {
				writeDemandError(w, r, http.StatusForbidden, mobile.ReasonLinkAuthorizationDeny, "Ссылка не может быть выпущена для этого пользователя.", false, []string{mobile.ReasonLinkAuthorizationDeny})
				return
			}
		}
		linkID, err := persistentid.New()
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEEPLINK_ISSUE_FAILED", "Ссылка временно недоступна.", true, nil)
			return
		}
		issuedAt := now().UTC()
		expiresAt := issuedAt.Add(deepLinkTTL)
		token, err := tokens.Issue(resource, linkID, issuedAt, expiresAt)
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, mobile.ReasonLinkInvalid, "Ресурс нельзя открыть безопасной ссылкой.", false, nil)
			return
		}
		writeJSON(w, http.StatusCreated, issueDeepLinkResponse{
			Token:        token,
			UniversalURL: "https://apgic.ru/l/" + url.PathEscape(token),
			ExpiresAt:    expiresAt,
		})
	})

	mux.HandleFunc("GET /v1/mobile/deep-links/resolve", func(w http.ResponseWriter, r *http.Request) {
		if store == nil || tokens == nil || tokenConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEEPLINK_UNAVAILABLE", "Безопасные ссылки временно недоступны.", false, nil)
			return
		}
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		claims, err := tokens.Parse(token, now().UTC())
		if err != nil {
			reason := mobile.ReasonLinkInvalid
			if errors.Is(err, mobile.ErrDeepLinkTokenExpired) {
				reason = mobile.ReasonLinkExpired
			}
			writeJSON(w, http.StatusOK, deepLinkResolutionResponse{Decision: "DENY", ReasonCode: reason})
			return
		}
		resource, found, err := store.DeepLinkResource(claims.Kind, claims.TargetID)
		switch {
		case err != nil:
			writeDemandError(w, r, http.StatusServiceUnavailable, "DEEPLINK_RESOURCE_UNAVAILABLE", "Ресурс ссылки временно недоступен.", true, nil)
			return
		case !found:
			writeJSON(w, http.StatusOK, deepLinkResolutionResponse{Decision: "DENY", ReasonCode: mobile.ReasonLinkInvalid})
			return
		}

		principal := authz.Principal{}
		if resource.AccessClass == mobile.LinkProtectedResource {
			identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
			if !ok {
				return
			}
			action := "deeplink.open." + strings.ToLower(string(resource.Kind))
			principal = authz.Principal{
				ID:       identityID,
				TenantID: resource.TenantID,
				Permissions: map[string]struct{}{
					action: {},
				},
			}
		}
		resolution := mobile.ResolveTrustedDeepLink(claims, resource, principal, now().UTC())
		decision := "DENY"
		if resolution.Allowed {
			decision = "ALLOW"
		}
		expiresAt := ""
		if !resolution.ExpiresAt.IsZero() {
			expiresAt = resolution.ExpiresAt.UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, deepLinkResolutionResponse{
			Decision:             decision,
			ReasonCode:           resolution.ReasonCode,
			CanonicalPath:        resolution.CanonicalPath,
			CanonicalWebFallback: resolution.WebFallback,
			ExpiresAt:            expiresAt,
		})
	})
}
