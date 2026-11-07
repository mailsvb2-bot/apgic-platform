package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func academyCloudHTTPError(w http.ResponseWriter, r *http.Request, err, fallback error) {
	status, code, message := 0, "", ""
	switch {
	case errors.Is(err, academy.ErrYandexReauth):
		status, code, message = http.StatusConflict, "ACADEMY_CLOUD_REAUTH_REQUIRED", "Доступ к Яндекс Диску истёк или отозван. Подключите Яндекс Диск заново."
	case errors.Is(err, academy.ErrYandexResourceMissing):
		status, code, message = http.StatusNotFound, "ACADEMY_CLOUD_FILE_NOT_FOUND", "Файл или папка не найдены на Яндекс Диске. Возможно, они перемещены или удалены."
	case errors.Is(err, academy.ErrYandexRateLimited):
		status, code, message = http.StatusServiceUnavailable, "ACADEMY_CLOUD_RATE_LIMITED", "Яндекс Диск временно ограничил число запросов. Повторите попытку через минуту."
	case errors.Is(err, academy.ErrYandexProviderDown), errors.Is(err, academy.ErrYandexTransport),
		errors.Is(err, context.DeadlineExceeded):
		status, code, message = http.StatusServiceUnavailable, "ACADEMY_CLOUD_PROVIDER_UNAVAILABLE", "Яндекс Диск не отвечает. Повторите попытку позже."
	}
	if status == 0 {
		academyHTTPError(w, r, fallback)
		return
	}
	writeDemandError(w, r, status, code, message, status >= 500, nil)
}

func academyCloudNotConnected(w http.ResponseWriter, r *http.Request) {
	writeDemandError(w, r, http.StatusConflict, "ACADEMY_CLOUD_NOT_CONNECTED", "Яндекс Диск не подключён. Подключите Яндекс Диск в настройках автора.", false, nil)
}

func academyCloudImportRejected(w http.ResponseWriter, r *http.Request) {
	writeDemandError(w, r, http.StatusUnprocessableEntity, "ACADEMY_CLOUD_MEDIA_REJECTED", "Файл не прошёл проверку: выберите видеофайл со своего Яндекс Диска. Если файл только что загружен, дождитесь окончания его обработки Диском и повторите.", false, nil)
}
