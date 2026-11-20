package httpapi

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"net/http"
	"strings"
)

type academyStandaloneDraftStore interface {
	AcademyCreateStandaloneDraft(actor, title, description string, requestIDs ...string) (academy.Course, error)
}

func registerAcademyStandaloneDraft(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("POST /v1/academy/drafts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		writer, ok := store.(academyStandaloneDraftStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			RequestID   string `json:"request_id"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		if len(input.Title) < 1 || len(input.Title) > 200 ||
			(input.RequestID != "" && !academyUUID(input.RequestID)) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		var course academy.Course
		var err error
		if input.RequestID == "" {
			course, err = writer.AcademyCreateStandaloneDraft(actor, input.Title, input.Description)
		} else {
			course, err = writer.AcademyCreateStandaloneDraft(actor, input.Title, input.Description, input.RequestID)
		}
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, course)
	})
}
