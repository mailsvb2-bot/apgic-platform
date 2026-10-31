package httpapi

import (
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Optional extension: the canonical Academy store remains the only writer.
// There is no second Identity, Commerce, Entitlement, or Booking owner.
type academyCohortStore interface {
	AcademyListCohorts(string) ([]academy.Cohort, error)
	AcademyCreateCohort(string, string, string, int, *time.Time, *time.Time) (academy.Cohort, error)
	AcademyJoinCohort(string, string, string) error
}

func registerAcademyCohorts(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/author/cohorts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		cohorts, ok := store.(academyCohortStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		items, err := cohorts.AcademyListCohorts(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"cohorts": items})
	})
	mux.HandleFunc("POST /v1/academy/versions/{versionID}/cohorts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		cohorts, ok := store.(academyCohortStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			Label              string     `json:"label"`
			Capacity           int        `json:"capacity"`
			StartsAt           *time.Time `json:"starts_at"`
			EnrollmentClosesAt *time.Time `json:"enrollment_closes_at"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		item, err := cohorts.AcademyCreateCohort(actor, r.PathValue("versionID"), input.Label, input.Capacity, input.StartsAt, input.EnrollmentClosesAt)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, item)
	})
	mux.HandleFunc("POST /v1/academy/cohorts/{cohortID}/join", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		cohorts, ok := store.(academyCohortStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			EnrollmentID string `json:"enrollment_id"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		if !academyUUID(input.EnrollmentID) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		if err := cohorts.AcademyJoinCohort(actor, r.PathValue("cohortID"), input.EnrollmentID); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]string{"state": "JOINED"})
	})
}
