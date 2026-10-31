package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCourseAccessStore interface {
	AcademyMyCourseAccess(string) ([]string, error)
}

// This is a read-only snapshot of effective canonical Commerce entitlements.
// It cannot be used to issue a grant or bypass the enrollment writer.
func registerAcademyCourseAccess(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/me/course-access", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		reader, ok := store.(academyCourseAccessStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		ids, err := reader.AcademyMyCourseAccess(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"course_ids": ids})
	})
}
