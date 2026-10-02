package httpapi

import (
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/remoteconfig"
)

type remoteConfigProvider func(time.Time) (remoteconfig.SignedEnvelope, error)

func registerMobileRemoteConfig(
	mux *http.ServeMux,
	provider remoteConfigProvider,
	now func() time.Time,
) {
	mux.HandleFunc("GET /v1/mobile/remote-config", func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "REMOTE_CONFIG_UNAVAILABLE", "Конфигурация приложения временно недоступна. Используется безопасный режим.", true, nil)
			return
		}
		envelope, err := provider(now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "REMOTE_CONFIG_UNAVAILABLE", "Конфигурация приложения временно недоступна. Используется безопасный режим.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, envelope)
	})
}
