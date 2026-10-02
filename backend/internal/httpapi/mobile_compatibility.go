package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
)

func registerMobileCompatibility(
	mux *http.ServeMux,
	policies map[clientcompat.Platform]clientcompat.Policy,
) {
	mux.HandleFunc("GET /v1/mobile/compatibility", func(w http.ResponseWriter, r *http.Request) {
		platform := clientcompat.Platform(strings.TrimSpace(r.URL.Query().Get("platform")))
		if platform != clientcompat.IOS && platform != clientcompat.Android {
			writeDemandError(w, r, http.StatusBadRequest, "CLIENT_PLATFORM_INVALID", "Платформа приложения указана некорректно.", false, nil)
			return
		}

		appVersion, err := clientcompat.ParseVersion(r.URL.Query().Get("app_version"))
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CLIENT_VERSION_INVALID", "Версия приложения указана некорректно.", false, nil)
			return
		}
		buildNumber, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("build_number")))
		if err != nil || buildNumber <= 0 {
			writeDemandError(w, r, http.StatusBadRequest, "CLIENT_BUILD_INVALID", "Номер сборки приложения указан некорректно.", false, nil)
			return
		}
		contractVersion := strings.TrimSpace(r.URL.Query().Get("contract_version"))
		if contractVersion == "" {
			writeDemandError(w, r, http.StatusBadRequest, "CLIENT_CONTRACT_INVALID", "Версия контракта приложения указана некорректно.", false, nil)
			return
		}

		policy, ok := policies[platform]
		if !ok {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_COMPATIBILITY_POLICY_UNAVAILABLE", "Проверка совместимости временно недоступна. Повторите попытку позже.", true, nil)
			return
		}
		decision, err := clientcompat.EvaluateClient(appVersion, buildNumber, contractVersion, policy)
		if err != nil {
			status := http.StatusServiceUnavailable
			reason := "CLIENT_COMPATIBILITY_POLICY_INVALID"
			message := "Проверка совместимости временно недоступна. Повторите попытку позже."
			retryable := true
			if errors.Is(err, clientcompat.ErrInvalidContract) || errors.Is(err, clientcompat.ErrInvalidBuild) {
				status = http.StatusBadRequest
				reason = "CLIENT_CONTRACT_INVALID"
				message = "Версия контракта или номер сборки приложения указаны некорректно."
				retryable = false
			}
			writeDemandError(w, r, status, reason, message, retryable, nil)
			return
		}
		writeJSON(w, http.StatusOK, decision)
	})
}
