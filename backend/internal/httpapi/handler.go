package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

type Options struct {
	CommitSHA                   string
	ReleaseTrack                string
	Surfaces                    []string
	LaunchConfig                launchconfig.Config
	ReadinessCheck              func(context.Context) error
	Demand                      *demand.Service
	LegalAcceptances            legal.AcceptanceStore
	Installations               mobile.InstallationStore
	Notifications               notification.MobileProjectionStore
	ClientMutations             mutation.Store
	DeepLinks                   deepLinkResourceStore
	DeepLinkSigningKey          []byte
	Specialists                 specialist.Store
	OrganizationAuth            organizationAuthorizationStore
	Organizations               organizationRuntimeStore
	ProductOwnership            productOwnershipStore
	Products                    organizationProductStore
	ClientSessionKey            []byte
	ClientCompatibilityPolicies map[clientcompat.Platform]clientcompat.Policy
	Now                         func() time.Time
}

type metaResponse struct {
	Service      string   `json:"service"`
	ReleaseTrack string   `json:"release_track"`
	Surfaces     []string `json:"surfaces"`
	CommitSHA    string   `json:"commit_sha,omitempty"`
	Time         string   `json:"time"`
}

type statusResponse struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func New(options Options) http.Handler {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.ReleaseTrack == "" {
		options.ReleaseTrack = "R0"
	}
	if len(options.Surfaces) == 0 {
		options.Surfaces = []string{"WEB", "PWA", "IOS", "ANDROID"}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !launchconfig.Ready(options.LaunchConfig) {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{
				Status:     "not_ready",
				ReasonCode: launchconfig.ReasonConfigRequired,
			})
			return
		}
		if options.ReadinessCheck != nil && options.ReadinessCheck(r.Context()) != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{
				Status:     "not_ready",
				ReasonCode: "STORAGE_UNAVAILABLE",
			})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
	})

	mux.HandleFunc("GET /v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, metaResponse{
			Service:      "apgic-api",
			ReleaseTrack: options.ReleaseTrack,
			Surfaces:     options.Surfaces,
			CommitSHA:    options.CommitSHA,
			Time:         options.Now().UTC().Format(time.RFC3339),
		})
	})

	var sessions *clientSessionManager
	var stepUp *stepUpManager
	var sessionConfigErr error
	if len(options.ClientSessionKey) > 0 {
		sessions, sessionConfigErr = newClientSessionManager(options.ClientSessionKey, options.Now)
		if sessionConfigErr == nil {
			stepUp, sessionConfigErr = newStepUpManager(options.ClientSessionKey, options.Now)
		}
	}
	registerDemand(mux, options.Demand, sessions, sessionConfigErr)
	registerLegalAcceptance(mux, options.LegalAcceptances, sessions, sessionConfigErr, options.Now)
	registerMobileCompatibility(mux, options.ClientCompatibilityPolicies)
	registerMobileInstallations(mux, options.Installations, sessions, sessionConfigErr, options.Now)
	registerMobileNotifications(mux, options.Notifications, sessions, sessionConfigErr)
	registerMobileCheckoutMutation(mux, options.Demand, options.ClientMutations, sessions, sessionConfigErr, options.Now)
	var deepLinkTokens *mobile.DeepLinkTokenManager
	var deepLinkTokenConfigErr error
	if len(options.DeepLinkSigningKey) > 0 {
		deepLinkTokens, deepLinkTokenConfigErr = mobile.NewDeepLinkTokenManager(options.DeepLinkSigningKey)
	}
	registerMobileDeepLinks(mux, options.DeepLinks, deepLinkTokens, deepLinkTokenConfigErr, sessions, sessionConfigErr, options.Now)
	registerSpecialist(mux, options.Specialists, sessions, sessionConfigErr)
	registerOrganizationAuthorization(mux, options.OrganizationAuth, sessions, sessionConfigErr, options.Now)
	registerOrganizationRuntime(mux, options.Organizations, sessions, sessionConfigErr)
	registerOrganizationProducts(mux, options.Products, sessions, sessionConfigErr, options.Now)
	registerHighRiskProductOwnership(mux, options.ProductOwnership, sessions, stepUp, sessionConfigErr, options.Now)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
