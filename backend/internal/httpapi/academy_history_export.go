package httpapi

import (
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The export is an owner-scoped read projection over canonical Academy.
// Assessment responses, evaluator evidence, payment credentials and other
// learner identities are deliberately excluded.
type academyHistoryReader interface {
	AcademyMyEnrollments(string) ([]academy.Enrollment, error)
	AcademyLearnerAttempts(string, string) ([]academy.Attempt, error)
}

type academyHistoryRecord struct {
	Enrollment academy.Enrollment `json:"enrollment"`
	Attempts   []academy.Attempt  `json:"attempts"`
}

func registerAcademyHistoryExport(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/me/history/export", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		reader, ok := store.(academyHistoryReader)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		enrollments, err := reader.AcademyMyEnrollments(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		records := make([]academyHistoryRecord, 0, len(enrollments))
		for _, enrollment := range enrollments {
			attempts, err := reader.AcademyLearnerAttempts(actor, enrollment.ID)
			if err != nil {
				academyHTTPError(w, r, err)
				return
			}
			records = append(records, academyHistoryRecord{Enrollment: enrollment, Attempts: attempts})
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Disposition", "attachment; filename=apgic-academy-history.json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": "academy-learning-export-v1",
			"generated_at":   time.Now().UTC(),
			"enrollments":    records,
		})
	})
}
