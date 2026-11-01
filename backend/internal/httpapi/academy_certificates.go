package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Certificate history is a read-only projection over the existing Academy
// owner. It never reissues credentials or bypasses revoked learning access.
func registerAcademyLearnerCertificates(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/me/certificates", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		reader, ok := store.(interface {
			AcademyMyCertificates(string) ([]academy.LearnerCertificate, error)
		})
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		certificates, err := reader.AcademyMyCertificates(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"certificates": certificates})
	})
}
