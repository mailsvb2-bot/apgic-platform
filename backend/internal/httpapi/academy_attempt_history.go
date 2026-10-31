package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyLearnerAttemptReader interface {
	AcademyLearnerAttempts(actor, enrollmentID string) ([]academy.Attempt, error)
}

func registerAcademyLearnerAttempts(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/attempts", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		reader, ok := store.(academyLearnerAttemptReader)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		attempts, err := reader.AcademyLearnerAttempts(identity, r.PathValue("enrollmentID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"attempts": attempts})
	})
}
