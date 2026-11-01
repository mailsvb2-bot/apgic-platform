package httpapi

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"net/http"
)

type academyCloudMediaStore interface {
	AcademyImportCloudMedia(string, []academy.CloudMedia) ([]academy.CloudMedia, error)
	AcademyAuthorCloudMedia(string) ([]academy.CloudMedia, error)
	AcademyAssignCloudMedia(string, string, string, []academy.MediaAssignment) error
	AcademyAuthorLessonMedia(string, string, string) ([]academy.LessonMediaView, error)
}

func registerAcademyCloudMedia(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("GET /v1/academy/author/cloud-media", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		owner, ok := store.(academyCloudMediaStore)
		if !ok {
			http.Error(w, "Unavailable", 503)
			return
		}
		out, err := owner.AcademyAuthorCloudMedia(actor)
		if err != nil {
			http.Error(w, "Unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"media": out})
	})
	mux.HandleFunc("POST /v1/academy/author/cloud-media/import", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		owner, ok := store.(academyCloudMediaStore)
		if !ok {
			http.Error(w, "Unavailable", 503)
			return
		}
		var body struct {
			Items []academy.CloudMedia `json:"items"`
		}
		if !academyRead(w, r, &body) {
			return
		}
		out, err := owner.AcademyImportCloudMedia(actor, body.Items)
		if err != nil {
			http.Error(w, "Import failed", 400)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 201, map[string]any{"media": out})
	})
	mux.HandleFunc("GET /v1/academy/author/courses/{courseID}/versions/{versionID}/media", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		owner, ok := store.(academyCloudMediaStore)
		if !ok {
			http.Error(w, "Unavailable", 503)
			return
		}
		out, err := owner.AcademyAuthorLessonMedia(actor, r.PathValue("courseID"), r.PathValue("versionID"))
		if err != nil {
			http.Error(w, "Unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"assignments": out})
	})
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions/{versionID}/media/assign", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		owner, ok := store.(academyCloudMediaStore)
		if !ok {
			http.Error(w, "Unavailable", 503)
			return
		}
		var body struct {
			Items []academy.MediaAssignment `json:"items"`
		}
		if !academyRead(w, r, &body) {
			return
		}
		if err := owner.AcademyAssignCloudMedia(actor, r.PathValue("courseID"), r.PathValue("versionID"), body.Items); err != nil {
			http.Error(w, "Assignment rejected", 409)
			return
		}
		writeJSON(w, 200, map[string]any{"state": "SAVED"})
	})
}
