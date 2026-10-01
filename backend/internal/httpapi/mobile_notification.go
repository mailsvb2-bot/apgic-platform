package httpapi

import (
	"net/http"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
)

func registerMobileNotifications(
	mux *http.ServeMux,
	store notification.MobileProjectionStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
) {
	mux.HandleFunc("GET /v1/mobile/notification-deliveries/{deliveryID}", func(w http.ResponseWriter, r *http.Request) {
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "NOTIFICATION_TRANSPORT_UNAVAILABLE", "Транспорт уведомлений временно недоступен.", true, nil)
			return
		}
		deliveryID := strings.TrimSpace(r.PathValue("deliveryID"))
		if deliveryID == "" || len(deliveryID) > 128 {
			writeDemandError(w, r, http.StatusBadRequest, "NOTIFICATION_DELIVERY_INVALID", "Идентификатор доставки некорректен.", false, nil)
			return
		}
		projection, found, err := store.MobileNotificationDelivery(r.Context(), identityID, deliveryID)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "NOTIFICATION_TRANSPORT_UNAVAILABLE", "Транспорт уведомлений временно недоступен.", true, nil)
			return
		}
		if !found {
			writeDemandError(w, r, http.StatusNotFound, "NOTIFICATION_DELIVERY_NOT_FOUND", "Уведомление не найдено.", false, nil)
			return
		}
		writeJSON(w, http.StatusOK, projection)
	})
}
