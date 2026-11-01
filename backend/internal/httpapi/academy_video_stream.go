package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyVideoStreamingStore interface {
	AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error)
	AcademyReadCloudCredential(actor, provider string) (string, error)
}

// Recheck enrollment and entitlement on every video HTTP request; never
// redirect the browser to an origin URL, even after a failed range request.
func registerAcademyVideoStream(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/videos/{mediaID}/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		owner, ok := store.(academyVideoStreamingStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		author, provider, path, err := owner.AcademyLearnerVideoSource(actor, r.PathValue("enrollmentID"), r.PathValue("lessonID"), r.PathValue("mediaID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		token, err := owner.AcademyReadCloudCredential(author, provider)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		download, err := (academy.YandexDiskClient{}).TemporaryDownload(ctx, token, path)
		cancel()
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		u, err := url.Parse(download)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, download, nil)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if value := r.Header.Get("Range"); value != "" {
			if len(value) > 128 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
				http.Error(w, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			request.Header.Set("Range", value)
		}
		transport := &http.Client{
			Transport:     &http.Transport{Proxy: nil, ResponseHeaderTimeout: 15 * time.Second, DisableCompression: true},
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}
		response, err := transport.Do(request)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		ct := response.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "application/octet-stream") {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		w.Header().Set("Content-Type", ct)
		if v := response.Header.Get("Content-Length"); v != "" {
			w.Header().Set("Content-Length", v)
		}
		if v := response.Header.Get("Content-Range"); v != "" {
			w.Header().Set("Content-Range", v)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	})
}
