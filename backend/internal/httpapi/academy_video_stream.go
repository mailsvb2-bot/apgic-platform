package httpapi

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyVideoStreamingStore interface {
	AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error)
	AcademyReadCloudCredential(actor, provider string) (string, error)
}

// Only the existing canonical PostgreSQL media owner may supply the verified
// file fingerprint; no client-provided path or checksum is trusted.
type academyVideoFingerprintStore interface {
	AcademyLearnerVideoFingerprint(author, provider, path, mediaID string) (string, string, int64, error)
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
		// Reject ambiguous or malformed browser Range headers before any
		// credential lookup or outbound cloud request.
		requestedRange, validRange := academyVideoRequestedRange(r.Header)
		if !validRange {
			http.Error(w, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
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
		fingerprints, ok := store.(academyVideoFingerprintStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		resourceID, digest, size, err := fingerprints.AcademyLearnerVideoFingerprint(author, provider, path, r.PathValue("mediaID"))
		if err != nil {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		token, err := owner.AcademyReadCloudCredential(author, provider)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		providerClient := academy.YandexDiskClient{}
		metadata, err := providerClient.VerifyVideo(ctx, token, path)
		if err != nil && academy.YandexProviderError(err) {
			cancel()
			academyCloudHTTPError(w, r, err, academy.ErrUnavailable)
			return
		}
		if err != nil || !academyVideoFingerprintMatches(metadata, resourceID, digest, size) {
			cancel()
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		download, err := providerClient.TemporaryDownload(ctx, token, path)
		cancel()
		if err != nil {
			academyCloudHTTPError(w, r, err, academy.ErrUnavailable)
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
		// A reconnected cloud account may point the same disk:/ path at a
		// different author-owned file. Bind the stream to the credential used
		// to resolve its original temporary URL, not only to the path.
		connectionAllowed := func() bool {
			return academyVideoCloudConnectionStillAllowed(owner, author, provider, token)
		}
		// The verified media row may be retired or replaced independently of
		// the enrollment and OAuth credential. Re-read its canonical binding
		// while bytes are flowing; a stale media identity must fail closed.
		fingerprintAllowed := func() bool {
			currentResource, currentDigest, currentSize, lookupErr := fingerprints.AcademyLearnerVideoFingerprint(author, provider, path, r.PathValue("mediaID"))
			return lookupErr == nil && currentResource == resourceID &&
				strings.EqualFold(currentDigest, digest) && currentSize == size
		}
		authorized := func() bool {
			return streamCtx.Err() == nil && allowed() && connectionAllowed() && fingerprintAllowed()
		}
		if !authorized() {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		// Revocation must interrupt not only a stalled cloud read but also a
		// blocked write to a slow learner. Canceling the upstream context alone
		// does not interrupt net/http ResponseWriter.Write.
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			academyWatchVideoEntitlement(streamCtx, func() {
				academyCancelRevokedVideoResponse(w, stopStream)
			}, time.Second, authorized)
		}()
		// The watcher must exit before ServeHTTP returns: it owns a reference
		// to ResponseWriter and must never touch it after the handler finishes.
		defer func() {
			stopStream()
			<-watchDone
		}()
		request, err := academyVideoUpstreamRequest(streamCtx, download, requestedRange)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		transport := academyVideoUpstreamClient()
		response, err := transport.Do(request)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			if !academyVideoRepresentationSizeMatches(response.StatusCode, response.Header, size) {
				academyHTTPError(w, r, academy.ErrUnavailable)
				return
			}
			value := response.Header.Get("Content-Range")
			// Header.Get hides additional Content-Range fields. Never accept
			// an ambiguous 416 from the cloud provider.
			if !academyVideoUnsatisfiedRangeHeadersAllowed(requestedRange, response.Header) {
				academyHTTPError(w, r, academy.ErrUnavailable)
				return
			}
			if !authorized() {
				academyHTTPError(w, r, academy.ErrEntitlement)
				return
			}
			w.Header().Set("Content-Range", value)
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if !academyVideoRepresentationSizeMatches(response.StatusCode, response.Header, size) ||
			!academyVideoUpstreamResponseAllowed(requestedRange, response.StatusCode, response.Header) {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		// Content-Range alone does not constrain a chunked upstream body.
		// Never forward bytes beyond the approved partial representation.
		videoBody, err := academyVideoBodyForLearner(response.StatusCode, response.Header, response.Body)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		if !allowed() {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		ct := response.Header.Get("Content-Type")
		// Reauthorize the source and credential immediately before committing
		// headers; either may have changed while the provider was resolving.
		if !authorized() {
			academyHTTPError(w, r, academy.ErrEntitlement)
			return
		}
		w.Header().Set("Content-Type", ct)
		if response.StatusCode == http.StatusPartialContent {
			// A chunked provider response has no Content-Length. Derive one
			// from the verified Content-Range so a short upstream body cannot
			// masquerade as a successful, complete HTML5 seek response.
			length, lengthErr := academyVideoPartialLength(response.Header.Get("Content-Range"))
			if lengthErr != nil {
				academyHTTPError(w, r, academy.ErrUnavailable)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		} else if v := response.Header.Get("Content-Length"); v != "" {
			w.Header().Set("Content-Length", v)
		}
		if v := response.Header.Get("Content-Range"); v != "" {
			w.Header().Set("Content-Range", v)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(response.StatusCode)
		// Bound idle provider body reads, not total video duration. An upstream
		// that stalls indefinitely after its headers must not pin a gateway
		// worker and keep polling PostgreSQL forever.
		idleBody := academyVideoIdleReader{source: videoBody, cancel: stopStream, timeout: 30 * time.Second}
		_ = academyCopyVideoAuthorized(w, idleBody, authorized)
	})
}

// A cloud path can be overwritten even without reconnecting OAuth.
// Before EVERY new stream/seek, prove the resource and its digest still
// match the file verified at import. Fail closed if metadata is unavailable.
func academyVideoFingerprintMatches(file academy.YandexDiskFile, resourceID, digest string, size int64) bool {
	return resourceID != "" && digest != "" && size > 0 &&
		file.ResourceID == resourceID && strings.EqualFold(file.SHA256, digest) && file.Size == size
}

// The provider's byte representation must have the SAME total length as
// the verified resource. Reject swapped or stale download URLs for 200/206/416.
func academyVideoRepresentationSizeMatches(status int, headers http.Header, size int64) bool {
	if size <= 0 {
		return false
	}
	if status == http.StatusOK {
		length, err := academyVideoFullLength(headers)
		return err == nil && length == size
	}
	value := headers.Get("Content-Range")
	var totalText string
	if status == http.StatusPartialContent {
		_, totalText, _ = strings.Cut(value, "/")
	} else if status == http.StatusRequestedRangeNotSatisfiable {
		totalText = strings.TrimPrefix(value, "bytes */")
	} else {
		return false
	}
	total, err := strconv.ParseInt(totalText, 10, 64)
	return err == nil && total == size
}

// academyVideoSourceStillAllowed binds every recheck to the original author,
// provider and source, not merely to a still-existing media path.
func academyVideoSourceStillAllowed(owner academyVideoStreamingStore, actor, enrollmentID, lessonID, mediaID, author, provider, path string) bool {
	currentAuthor, currentProvider, currentPath, err := owner.AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID)
	return err == nil && currentAuthor == author && currentProvider == provider && currentPath == path
}

// academyVideoCloudConnectionStillAllowed also invalidates a running
// stream if the author reconnects a different cloud account while the old
// temporary URL remains valid. Compare credentials server-side only; never
// include them in learner responses, logs or error strings.
func academyVideoCloudConnectionStillAllowed(owner academyVideoStreamingStore, author, provider, originalToken string) bool {
	currentToken, err := owner.AcademyReadCloudCredential(author, provider)
	return err == nil && originalToken != "" && subtle.ConstantTimeCompare([]byte(currentToken), []byte(originalToken)) == 1
}

// academyVideoRequestedRange accepts at most one canonical HTTP Range field.
// net/http Header.Get hides additional values supplied by a client.
func academyVideoRequestedRange(headers http.Header) (string, bool) {
	values := headers.Values("Range")
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 || !academyVideoSingleRange(values[0]) {
		return "", false
	}
	return values[0], true
}

func academyVideoSingleRange(value string) bool {
	if len(value) > 128 || !strings.HasPrefix(value, "bytes=") {
		return false
	}
	spec := strings.TrimPrefix(value, "bytes=")
	if strings.Count(spec, "-") != 1 || strings.ContainsAny(spec, ",+ \t") {
		return false
	}
	first, last, _ := strings.Cut(spec, "-")
	if first == "" {
		if last == "" {
			return false
		}
		suffix, err := strconv.ParseUint(last, 10, 64)
		return err == nil && suffix > 0
	}
	start, err := strconv.ParseUint(first, 10, 64)
	if err != nil {
		return false
	}
	if last == "" {
		return true
	}
	finish, err := strconv.ParseUint(last, 10, 64)
	return err == nil && finish >= start
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

// academyCancelRevokedVideoResponse closes both directions of the live media
// stream. The deadline interrupts a learner-side Write blocked on a slow or
// stalled connection; upstream cancellation alone cannot stop that Write.
func academyCancelRevokedVideoResponse(w http.ResponseWriter, cancel context.CancelFunc) {
	cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now())
}

// academyVideoUpstreamClient is the single hardened media-download transport.
// TCP and response-header timeouts do not cover a stalled TLS handshake.
// Bound the handshake and header bytes without setting a whole-response
// timeout (large, authorized video streams can legitimately take longer).
func academyVideoUpstreamClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives:      true,
			Proxy:                  nil,
			DialContext:            (&net.Dialer{Timeout: 10 * time.Second, ControlContext: academyVideoDialControl}).DialContext,
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  15 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
			DisableCompression:     true,
		},
		CheckRedirect: academy.YandexMediaCheckRedirect,
	}
}
