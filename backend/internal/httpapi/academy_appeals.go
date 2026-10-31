package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyAppealStore interface {
	AcademySubmitAppeal(string, string, string) (academy.Appeal, error)
	AcademyMyAppeals(string) ([]academy.Appeal, error)
}

func registerAcademyAppeals(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("POST /v1/academy/review/appeals/{appealID}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		id := r.PathValue("appealID")
		if !academyUUID(id) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		reviewer, ok := store.(interface {
			AcademyReviewAppeal(string, string, string, string) error
		})
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			Decision    string `json:"decision"`
			EvidenceRef string `json:"evidence_ref"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		if err := reviewer.AcademyReviewAppeal(actor, id, input.Decision, input.EvidenceRef); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]string{"state": input.Decision})
	})
	mux.HandleFunc("POST /v1/academy/attempts/{attemptID}/appeals", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		attemptID := r.PathValue("attemptID")
		if !academyUUID(attemptID) {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		appeals, ok := store.(academyAppealStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input struct {
			Reason string `json:"reason"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		record, err := appeals.AcademySubmitAppeal(actor, attemptID, input.Reason)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, record)
	})
	mux.HandleFunc("GET /v1/academy/me/appeals", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		appeals, ok := store.(academyAppealStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		records, err := appeals.AcademyMyAppeals(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"appeals": records})
	})
}
