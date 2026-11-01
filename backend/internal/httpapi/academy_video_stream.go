package httpapi

import (
	"context"
	"net/http"
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
		if !academy.ValidateYandexDownloadURL(download) {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		streamCtx, stopStream := context.WithCancel(r.Context())
		defer stopStream()
		// The same canonical access check guards startup, byte copying and
		// idle upstream reads. A stalled provider must not outlive revocation.
		allowed := func() bool {
			return academyVideoSourceStillAllowed(owner, actor,
				r.PathValue("enrollmentID"), r.PathValue("lessonID"), r.PathValue("mediaID"), author, provider, path)
		}
		if !allowed() {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		go academyWatchVideoEntitlement(streamCtx, stopStream, time.Second, allowed)
		request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, download, nil)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if value := r.Header.Get("Range"); value != "" {
			if !academyVideoSingleRange(value) {
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
		if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			value := response.Header.Get("Content-Range")
			if !academyVideoUnsatisfiedRangeAllowed(r.Header.Get("Range"), value) {
				academyHTTPError(w, r, academy.ErrUnavailable)
				return
			}
			if !allowed() {
				academyHTTPError(w, r, academy.ErrEntitlement)
				return
			}
			w.Header().Set("Content-Range", value)
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if !academyVideoUpstreamResponseAllowed(r.Header.Get("Range"), response.StatusCode, response.Header) {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if !allowed() {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		ct := response.Header.Get("Content-Type")
		// Reauthorize immediately before committing response headers; entitlement
		// might have been revoked while the cloud provider was resolving.
		if !allowed() {
			academyHTTPError(w, r, academy.ErrEntitlement)
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
		_ = academyCopyVideoAuthorized(w, response.Body, allowed, 512<<10, time.Second)
	})
}

// academyVideoSourceStillAllowed binds every recheck to the original author,
// provider and source, not merely to a still-existing media path.
func academyVideoSourceStillAllowed(owner academyVideoStreamingStore, actor, enrollmentID, lessonID, mediaID, author, provider, path string) bool {
	currentAuthor, currentProvider, currentPath, err := owner.AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID)
	return err == nil && currentAuthor == author && currentProvider == provider && currentPath == path
}

func academyVideoSingleRange(value string) bool {
	if len(value) > 128 || !strings.HasPrefix(value, "bytes=") {
		return false
	}
	spec := strings.TrimPrefix(value, "bytes=")
	if strings.Count(spec, "-") != 1 || strings.Contains(spec, ",") {
		return false
	}
	parts := strings.SplitN(spec, "-", 2)
	if parts[0] == "" && parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

// academyWatchVideoEntitlement watches idle, stalled and active video streams.
// A revocation cancels the HTTP request even if the provider stops sending.
func academyWatchVideoEntitlement(ctx context.Context, cancel context.CancelFunc, interval time.Duration, allowed func() bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !allowed() {
				cancel()
				return
			}
		}
	}
}
