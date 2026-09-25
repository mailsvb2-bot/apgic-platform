package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
)

type Options struct {
	CommitSHA    string
	ReleaseTrack string
	Surfaces     []string
	LaunchConfig launchconfig.Config
	Demand       *demand.Service
	Now          func() time.Time
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

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !launchconfig.Ready(options.LaunchConfig) {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{
				Status:     "not_ready",
				ReasonCode: launchconfig.ReasonConfigRequired,
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

	registerDemand(mux, options.Demand)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
