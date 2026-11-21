package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCloudBrowser interface {
	AcademyBrowseYandexDisk(actor, path string, offset, limit int) ([]academy.YandexDiskFile, int, error)
}

// Disconnect uses the existing canonical PostgreSQL credential owner.
// Removing the connection invalidates all current-generation media and streams.
type academyCloudDisconnector interface {
	AcademyDisconnectCloud(actor, provider string) error
}

// academyCloudBrowserQuery rejects ambiguous or malformed folder and page
// selectors before the author's OAuth credential is read. URL.Query().Get()
// silently picks the first duplicate and ignores invalid percent escapes.
func academyCloudBrowserQuery(rawQuery string) (string, int, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", 0, academy.ErrInvalid
	}
	for key := range values {
		if key != "path" && key != "offset" {
			return "", 0, academy.ErrInvalid
		}
	}
	path := "disk:/"
	if selected, ok := values["path"]; ok {
		if len(selected) != 1 || !academy.ValidYandexDiskPath(selected[0]) {
			return "", 0, academy.ErrInvalid
		}
		path = selected[0]
	}
	offset := 0
	if page, ok := values["offset"]; ok {
		if len(page) != 1 || len(page[0]) == 0 || len(page[0]) > 7 {
			return "", 0, academy.ErrInvalid
		}
		for _, digit := range page[0] {
			if digit < '0' || digit > '9' {
				return "", 0, academy.ErrInvalid
			}
		}
		offset, err = strconv.Atoi(page[0])
		if err != nil || offset > 1000000 {
			return "", 0, academy.ErrInvalid
		}
	}
	return path, offset, nil
}

// A same-origin authenticated file picker API. It returns metadata only.
func registerAcademyCloudBrowser(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("POST /v1/academy/author/cloud/yandex/disconnect", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		// Credential-revoking operations require a first-party browser POST,
		// not a cross-site form or a forged cookie-bearing request.
		if !accountPairOrigin(r) {
			academyHTTPError(w, r, academy.ErrDenied)
			return
		}
		var input struct {
			Confirm string `json:"confirm"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		if input.Confirm != "DISCONNECT_YANDEX_DISK" {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		owner, ok := store.(academyCloudDisconnector)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if err := owner.AcademyDisconnectCloud(actor, "YANDEX_DISK"); err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
	})
	mux.HandleFunc("GET /v1/academy/author/cloud/yandex/files", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		path, offset, err := academyCloudBrowserQuery(r.URL.RawQuery)
		if err != nil {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		reader, ok := store.(academyCloudBrowser)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		files, total, err := reader.AcademyBrowseYandexDisk(actor, path, offset, 100)
		if errors.Is(err, academy.ErrDenied) {
			academyCloudNotConnected(w, r)
			return
		}
		if err != nil {
			academyCloudHTTPError(w, r, err, academy.ErrUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"files": files, "total": total, "offset": offset})
	})
}
