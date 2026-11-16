package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Canon v7 assigns media-byte delivery and playback attestation to a
// versioned CDN_PROVIDER capability. No APGIC Go Yandex byte proxy is part of
// the production binary. Until a real certified CDN adapter is wired, learner
// video delivery fails closed.
func registerAcademyVideoStream(mux *http.ServeMux, _ academyStore, _ *clientSessionManager, _ error) {
	mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/videos/{mediaID}/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		academyHTTPError(w, r, academy.ErrUnavailable)
	})
}
