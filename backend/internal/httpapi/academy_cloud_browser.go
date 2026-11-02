package httpapi

import (
	"net/http"
	"strconv"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCloudBrowser interface {
	AcademyBrowseYandexDisk(actor, path string, offset, limit int) ([]academy.YandexDiskFile, int, error)
}

// A same-origin authenticated file picker API. It returns metadata only.
func registerAcademyCloudBrowser(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("GET /v1/academy/author/cloud/yandex/files", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		reader, ok := store.(academyCloudBrowser)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			path = "disk:/"
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") == "" {
			offset = 0
			err = nil
		}
		if err != nil || offset < 0 {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		files, total, err := reader.AcademyBrowseYandexDisk(actor, path, offset, 100)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"files": files, "total": total, "offset": offset})
	})
}
