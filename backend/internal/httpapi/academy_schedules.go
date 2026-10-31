package httpapi

import (
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyScheduleWriter interface {
	AcademySetLessonSchedule(string, string, *time.Time, *time.Time) error
	AcademySetAssessmentLimits(string, string, int, *time.Time) error
}

func registerAcademySchedules(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("POST /v1/academy/lessons/{lessonID}/schedule", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		id := r.PathValue("lessonID")
		if !academyUUID(id) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		writer, ok := store.(academyScheduleWriter)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var body struct {
			ReleaseAt  *time.Time `json:"release_at"`
			DeadlineAt *time.Time `json:"deadline_at"`
		}
		if !academyRead(w, r, &body) {
			return
		}
		if err := writer.AcademySetLessonSchedule(actor, id, body.ReleaseAt, body.DeadlineAt); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]string{"state": "UPDATED"})
	})
	mux.HandleFunc("POST /v1/academy/lessons/{lessonID}/assessment-limits", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		id := r.PathValue("lessonID")
		if !academyUUID(id) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		writer, ok := store.(academyScheduleWriter)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var body struct {
			MaxAttempts int        `json:"max_attempts"`
			DeadlineAt  *time.Time `json:"submission_deadline"`
		}
		if !academyRead(w, r, &body) {
			return
		}
		if err := writer.AcademySetAssessmentLimits(actor, id, body.MaxAttempts, body.DeadlineAt); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]string{"state": "UPDATED"})
	})
}
