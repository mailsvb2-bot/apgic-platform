package httpapi

import (
	"context"
	"crypto/subtle"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The single canonical recheck cadence used by real HTTP video streams and
// cross-process PostgreSQL revocation evidence. A COMMIT can take up to one
// polling period to be observed; this is not zero-latency push delivery.
const academyVideoEntitlementRecheckInterval = time.Second

type academyVideoStreamingStore interface {
	AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error)
	AcademyReadCloudCredential(actor, provider string) (string, error)
}

// Only the existing canonical PostgreSQL media owner may supply the verified
// file fingerprint; no client-provided path or checksum is trusted.
type academyVideoFingerprintStore interface {
	AcademyLearnerVideoFingerprint(author, provider, path, mediaID string) (string, string, int64, error)
}

// First-party delivery facts are written by the existing Academy store only
// for verified bytes flushed to the learner. A range or interrupted transfer
// cannot independently complete a lesson: all intervals must cover every file.
type academyDirectVideoDeliveryWriter interface {
	AcademyRecordDirectVideoDelivery(actor, enrollmentID, lessonID, mediaID, digest string, length, start, end int64) error
}

// This is the canonical direct Yandex-to-APGIC learner media gateway.
// The client-injected entrypoint is also used by transport and PostgreSQL
// integration tests: production and tests execute the same authorization,
// source fingerprint, Range, revocation and bounded-copy logic.
// registerAcademyVideoStream uses the same trusted gateway as the provider
// integration tests. OAuth credentials and temporary URLs never reach clients.
func registerAcademyVideoStream(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mode, err := academy.ConfiguredVideoDeliveryMode()
	if err != nil || mode != academy.VideoDeliveryDirect {
		// Preserve all CDN connector contracts and evidence storage, but do
		// not silently fall back to Yandex or advertise a CDN stream before
		// the production CDN_PROVIDER adapter is registered and certified.
		// The one public media route stays closed on unknown/CDN modes.
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if _, ok := academyIdentity(w, r, sessions, sessionErr); !ok {
				return
			}
			academyHTTPError(w, r, academy.ErrUnavailable)
		}
		mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/videos/{mediaID}/stream", unavailable)
		mux.HandleFunc("GET /v1/academy/author/versions/{versionID}/lessons/{lessonID}/videos/{mediaID}/stream", unavailable)
		return
	}
	registerAcademyVideoStreamWithClients(mux, store, sessions, sessionErr, academy.YandexDiskClient{}, academyVideoUpstreamClient())
}

func registerAcademyVideoStreamWithClients(
	mux *http.ServeMux,
	store academyStore,
	sessions *clientSessionManager,
	sessionErr error,
	providerClient academy.YandexDiskClient,
	upstreamClient *http.Client,
) {
	if upstreamClient == nil {
		upstreamClient = academyVideoUpstreamClient()
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		deadline := newAcademyVideoResponseDeadline(w)
		deadline.extend(academyVideoPreflightTimeout)
		// Reject ambiguous or malformed browser Range headers before any
		// credential lookup or outbound cloud request.
		requestedRange, validRange := academyVideoRequestedRange(r.Header)
		if !validRange {
			// 416 is reserved for a syntactically valid range that cannot be
			// satisfied by the selected representation. Malformed, duplicate
			// or multi-range input is rejected before metadata lookup as 400;
			// returning 416 here would omit the required Content-Range size.
			http.Error(w, "Invalid range", http.StatusBadRequest)
			return
		}
		owner, ok := store.(academyVideoStreamingStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		preview := r.PathValue("versionID") != ""
		source := func() (string, string, string, error) {
			if preview {
				reader, ok := store.(interface {
					AcademyAuthorVideoSource(actor, versionID, lessonID, mediaID string) (string, string, string, error)
				})
				if !ok {
					return "", "", "", academy.ErrUnavailable
				}
				return reader.AcademyAuthorVideoSource(actor, r.PathValue("versionID"), r.PathValue("lessonID"), r.PathValue("mediaID"))
			}
			return owner.AcademyLearnerVideoSource(actor, r.PathValue("enrollmentID"), r.PathValue("lessonID"), r.PathValue("mediaID"))
		}
		author, provider, path, err := source()
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
			currentAuthor, currentProvider, currentPath, err := source()
			return err == nil && currentAuthor == author && currentProvider == provider && currentPath == path
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
			return academyVideoFingerprintStillAllowed(fingerprints, author, provider, path,
				r.PathValue("mediaID"), resourceID, digest, size)
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
				stopStream()
				deadline.revoke()
			}, academyVideoEntitlementRecheckInterval, authorized)
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
		// Explicitly request the original representation. Besides informing the
		// provider, this disables net/http automatic gzip decoding even when a
		// caller injects a default Transport rather than our hardened client.
		// Without this, transparent decompression strips Content-Encoding and
		// could bypass the response-side identity check below.
		request.Header.Set("Accept-Encoding", "identity")
		response, err := upstreamClient.Do(request)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		defer response.Body.Close()
		// A provider-side content coding changes the byte representation used
		// by Range and the imported SHA-256/size. Never forward or credit an
		// encoded representation as the verified MP4, even when the cloud
		// supplies otherwise plausible Content-Length/Content-Range headers.
		if !academyVideoOriginalRepresentation(response) {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		// Canceling a request context is sufficient for net/http transports,
		// but a custom or stalled upstream body is not required to notice it.
		// Explicitly close the actual provider reader on revocation, client
		// disconnect and idle timeout so a blocked Read cannot pin this handler.
		stopBodyClose := academyCloseVideoBodyOnCancel(streamCtx, response.Body)
		defer stopBodyClose()
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
		deadline.extend(academyVideoWriteIdleTimeout)
		w.WriteHeader(response.StatusCode)
		// Bound idle provider body reads, not total video duration. An upstream
		// that stalls indefinitely after its headers must not pin a gateway
		// worker and keep polling PostgreSQL forever.
		idleBody := academyVideoIdleReader{source: videoBody, cancel: stopStream, timeout: 30 * time.Second}
		// If the per-write canonical check detects revocation before the
		// periodic watcher does, close the live learner response immediately.
		// Canceling the upstream alone does not interrupt a blocked client
		// write; the response deadline also prevents further buffered output.
		learner := &academyVideoLearnerWriter{w: w, deadline: deadline}
		copyErr := academyCopyVideoAuthorized(learner, idleBody, authorized)
		if copyErr == errAcademyVideoRevoked {
			stopStream()
			deadline.revoke()
			return
		}
		// Author preview never creates learner delivery evidence or progress.
		if preview {
			return
		}
		// Only flushed bytes count. The SQL owner checks entitlement and
		// every assigned media file before permitting lesson completion.
		start, end, valid := academyDeliveredVideoInterval(response.StatusCode, response.Header, size)
		if !valid {
			return
		}
		// io.Copy may return nil after a premature EOF even when the upstream
		// promised a longer Content-Length or Content-Range. Never credit bytes
		// that were not actually flushed to the learner.
		if copyErr != nil && (deadline.wasRevoked() || !authorized()) {
			return
		}
		if !authorized() {
			return
		}
		end = academyVideoDeliveredEnd(start, end, learner.delivered)
		if end <= start {
			return
		}
		if writer, ok := store.(academyDirectVideoDeliveryWriter); ok {
			_ = writer.AcademyRecordDirectVideoDelivery(actor,
				r.PathValue("enrollmentID"), r.PathValue("lessonID"),
				r.PathValue("mediaID"), digest, size, start, end)
		}
	}
	mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/videos/{mediaID}/stream", handler)
	mux.HandleFunc("GET /v1/academy/author/versions/{versionID}/lessons/{lessonID}/videos/{mediaID}/stream", handler)
}

// academyDeliveredVideoInterval translates an already validated upstream
// representation into one half-open byte interval for the Academy owner.
// No 416 or malformed/overflowing Content-Range can create progress.
func academyDeliveredVideoInterval(status int, headers http.Header, size int64) (int64, int64, bool) {
	if size <= 0 {
		return 0, 0, false
	}
	if status == http.StatusOK {
		if !academyVideoRepresentationSizeMatches(status, headers, size) {
			return 0, 0, false
		}
		return 0, size, true
	}
	if status != http.StatusPartialContent || !academyVideoRepresentationSizeMatches(status, headers, size) {
		return 0, 0, false
	}
	span, _, ok := strings.Cut(strings.TrimPrefix(headers.Get("Content-Range"), "bytes "), "/")
	if !ok {
		return 0, 0, false
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok {
		return 0, 0, false
	}
	start, errStart := strconv.ParseInt(startText, 10, 64)
	endInclusive, errEnd := strconv.ParseInt(endText, 10, 64)
	if errStart != nil || errEnd != nil || start < 0 || endInclusive < start || endInclusive >= size {
		return 0, 0, false
	}
	return start, endInclusive + 1, true
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

// Re-read the canonical verified media row during an active stream. Revocation,
// OAuth generation changes, and media replacement must all fail closed.
func academyVideoFingerprintStillAllowed(store academyVideoFingerprintStore, author, provider, path, mediaID, resourceID, digest string, size int64) bool {
	currentResource, currentDigest, currentSize, err := store.AcademyLearnerVideoFingerprint(author, provider, path, mediaID)
	return err == nil && currentResource == resourceID &&
		strings.EqualFold(currentDigest, digest) && currentSize == size
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

// academyVideoOriginalRepresentation also rejects bodies transparently decoded by
// a custom Go RoundTripper. In that case net/http may remove Content-Encoding,
// leaving only Response.Uncompressed as evidence that the verified bytes changed.
func academyVideoOriginalRepresentation(response *http.Response) bool {
	return response != nil && !response.Uncompressed && academyVideoIdentityEncoding(response.Header)
}

// academyVideoIdentityEncoding rejects compressed or ambiguous upstream bodies.
// Verified media fingerprints and HTTP byte ranges apply to the original
// MP4 representation, not to a provider-selected content coding.
func academyVideoIdentityEncoding(headers http.Header) bool {
	// Header.Values canonicalizes the lookup key, but a custom RoundTripper
	// can construct Response.Header directly with noncanonical casing. Scan
	// every key to avoid treating a lowercase or duplicate coding as absent.
	var values []string
	seen := false
	for key, fields := range headers {
		if strings.EqualFold(key, "Content-Encoding") {
			// A custom transport can supply a present but empty field slice.
			// Treat that malformed representation as ambiguous, not absent.
			if seen || len(fields) != 1 {
				return false
			}
			seen = true
			values = fields
		}
	}
	if !seen {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(values[0]), "identity")
}

// academyVideoRequestedRange accepts at most one canonical HTTP Range field.
// net/http Header.Get hides additional values supplied by a client.
func academyVideoRequestedRange(headers http.Header) (string, bool) {
	// Inspect raw keys: injected transports and direct handler callers can
	// supply noncanonical casing, which Header.Values("Range") would miss.
	// Never forward a hidden second Range to the upstream provider.
	var value string
	seen := false
	for key, fields := range headers {
		if !strings.EqualFold(key, "Range") {
			continue
		}
		if seen || len(fields) != 1 {
			return "", false
		}
		seen = true
		value = fields[0]
	}
	if !seen {
		return "", true
	}
	if !academyVideoSingleRange(value) {
		return "", false
	}
	return value, true
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
		suffix, err := strconv.ParseInt(last, 10, 64)
		return err == nil && suffix > 0
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		return false
	}
	if last == "" {
		return true
	}
	finish, err := strconv.ParseInt(last, 10, 64)
	return err == nil && finish >= start
}

// academyCloseVideoBodyOnCancel ensures the actual external body is closed
// even if an upstream RoundTripper ignores request-context cancellation.
// This is shared by the real gateway and its HTTP transport regression.
func academyCloseVideoBodyOnCancel(ctx context.Context, body io.Closer) func() bool {
	return context.AfterFunc(ctx, func() {
		_ = body.Close()
	})
}

// academyWatchVideoEntitlement watches idle, stalled and active video streams.
// A revocation cancels the HTTP request even if the provider stops sending.
func academyWatchVideoEntitlement(ctx context.Context, cancel context.CancelFunc, interval time.Duration, allowed func() bool) {
	// Check before waiting for the first tick: access may have been revoked
	// between the handler's initial authorization and watcher startup.
	if ctx.Err() != nil {
		return
	}
	if !allowed() {
		cancel()
		return
	}
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

type academyCountingVideoReader struct {
	reader io.Reader
	reads  int
}

func (r *academyCountingVideoReader) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

// academyVideoDeliveredEnd clamps credited progress to bytes actually
// flushed, including a clean EOF from a truncated upstream response.
func academyVideoDeliveredEnd(start, end, delivered int64) int64 {
	if delivered <= 0 || end <= start {
		return start
	}
	if delivered >= end-start {
		return end
	}
	return start + delivered
}
