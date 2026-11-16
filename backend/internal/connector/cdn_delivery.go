package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalidCDNPlaybackStream = errors.New("invalid CDN playback stream")

// CDNPlaybackByteRange is one normalized inclusive byte interval. Multi-range
// requests are intentionally outside this provider contract; open-ended and
// suffix ranges are normalized to this representation before provider I/O.
type CDNPlaybackByteRange struct {
	Start int64
	End   int64
}

func (r CDNPlaybackByteRange) Validate(contentLength int64) error {
	if contentLength <= 0 || r.Start < 0 || r.End < r.Start || r.End >= contentLength {
		return ErrInvalidCDNPlaybackStream
	}
	return nil
}

// CDNPlaybackRangeSelection is the gateway's decision for one HTTP Range
// header. A 416 decision includes the only representation metadata safe to
// return: "bytes */<verified content length>". Invalid syntax is an error and
// must be rejected as 400 before provider I/O.
type CDNPlaybackRangeSelection struct {
	StatusCode   int
	ByteRange    *CDNPlaybackByteRange
	ContentRange string
}

// SelectCDNPlaybackRange parses exactly zero or one Range field value against
// the immutable imported media length. It never trusts a provider response to
// decide whether a learner's range is satisfiable.
//
// Semantics:
//   - no Range header: 200 and a nil ByteRange;
//   - one satisfiable bytes range: 206 and a normalized inclusive interval;
//   - a syntactically valid but unsatisfiable range: 416 and bytes */length;
//   - duplicate, malformed, multi-range, signed, whitespace-padded or
//     overflowing input: ErrInvalidCDNPlaybackRequest (gateway maps to 400).
//
// Open-ended and suffix ranges are normalized here so the provider sees only
// a single bounded inclusive interval. An end beyond EOF is clamped to EOF.
func SelectCDNPlaybackRange(values []string, contentLength int64) (CDNPlaybackRangeSelection, error) {
	if contentLength <= 0 {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	if len(values) == 0 {
		return CDNPlaybackRangeSelection{StatusCode: http.StatusOK}, nil
	}
	if len(values) != 1 {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	value := values[0]
	if !strings.HasPrefix(value, "bytes=") || strings.ContainsAny(value, "\r\n") {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	spec := strings.TrimPrefix(value, "bytes=")
	if spec == "" || strings.Contains(spec, ",") || strings.Count(spec, "-") != 1 {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	startText, endText, _ := strings.Cut(spec, "-")
	if startText == "" {
		suffixLength, ok := cdnPlaybackRangeDecimal(endText)
		if !ok || suffixLength == 0 {
			return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
		}
		start := int64(0)
		if suffixLength < contentLength {
			start = contentLength - suffixLength
		}
		end := contentLength - 1
		return cdnPlaybackRangePartial(start, end, contentLength), nil
	}

	start, ok := cdnPlaybackRangeDecimal(startText)
	if !ok {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	if endText == "" {
		if start >= contentLength {
			return cdnPlaybackRangeUnsatisfied(contentLength), nil
		}
		return cdnPlaybackRangePartial(start, contentLength-1, contentLength), nil
	}
	end, ok := cdnPlaybackRangeDecimal(endText)
	if !ok || end < start {
		return CDNPlaybackRangeSelection{}, ErrInvalidCDNPlaybackRequest
	}
	if start >= contentLength {
		return cdnPlaybackRangeUnsatisfied(contentLength), nil
	}
	if end >= contentLength {
		end = contentLength - 1
	}
	return cdnPlaybackRangePartial(start, end, contentLength), nil
}

func cdnPlaybackRangeDecimal(value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func cdnPlaybackRangePartial(start, end, contentLength int64) CDNPlaybackRangeSelection {
	return CDNPlaybackRangeSelection{
		StatusCode: http.StatusPartialContent,
		ByteRange:  &CDNPlaybackByteRange{Start: start, End: end},
		ContentRange: fmt.Sprintf(
			"bytes %d-%d/%d", start, end, contentLength,
		),
	}
}

func cdnPlaybackRangeUnsatisfied(contentLength int64) CDNPlaybackRangeSelection {
	return CDNPlaybackRangeSelection{
		StatusCode:   http.StatusRequestedRangeNotSatisfiable,
		ContentRange: fmt.Sprintf("bytes */%d", contentLength),
	}
}

// CDNPlaybackStream is the only data a provider may return to the APGIC
// gateway for browser delivery. The body is an opaque stream; URLs, origin
// hosts, bearer tokens, cookies and arbitrary upstream headers are excluded.
type CDNPlaybackStream struct {
	StatusCode    int
	ContentType   string
	ContentLength int64
	ContentRange  string
	Body          io.ReadCloser
}

// cdnPlaybackContextBody makes request cancellation close the provider's
// already-open body. This is the final transport safety net after the HTTP
// gateway's canonical Entitlement watcher cancels the request context.
type cdnPlaybackContextBody struct {
	body     io.ReadCloser
	ctx      context.Context
	done     chan struct{}
	once     sync.Once
	closeErr error
	cancel   context.CancelFunc
}

// bindCDNPlaybackBody enforces the validated representation length as well as
// cancellation. A misbehaving adapter cannot send bytes beyond the declared
// media/range boundary to a learner, even if its body is longer than metadata.
func bindCDNPlaybackBody(ctx context.Context, body io.ReadCloser, contentLength int64, expiresAt ...time.Time) io.ReadCloser {
	var cancel context.CancelFunc
	if len(expiresAt) > 0 {
		ctx, cancel = context.WithDeadline(ctx, expiresAt[0])
	}
	bound := &cdnPlaybackContextBody{
		body: &cdnPlaybackBoundedBody{reader: &io.LimitedReader{R: body, N: contentLength}, closer: body}, ctx: ctx, done: make(chan struct{}), cancel: cancel,
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = bound.Close()
		case <-bound.done:
		}
	}()
	return bound
}

type cdnPlaybackBoundedBody struct {
	reader     *io.LimitedReader
	closer     io.Closer
	noProgress int
}

// A truncated provider body must not look like a complete video representation.
// The length bound still prevents delivery of any extra provider bytes.
func (b *cdnPlaybackBoundedBody) Read(p []byte) (int, error) {
	if b.reader.N == 0 {
		return 0, io.EOF
	}
	n, err := b.reader.Read(p)
	if err == io.EOF && b.reader.N > 0 {
		return n, io.ErrUnexpectedEOF
	}
	// A faulty provider must not keep the HTTP response spinning forever by
	// repeatedly returning (0, nil). Follow io.ReadAll's no-progress bound.
	if n == 0 && err == nil && len(p) > 0 {
		b.noProgress++
		if b.noProgress >= 100 {
			// A permanently stalled adapter is unusable. Release its upstream
			// resources immediately rather than waiting for an HTTP caller to close.
			_ = b.closer.Close()
			return 0, io.ErrNoProgress
		}
	} else {
		b.noProgress = 0
	}
	return n, err
}

func (b *cdnPlaybackBoundedBody) Close() error { return b.closer.Close() }

func (b *cdnPlaybackContextBody) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := b.body.Read(p)
	if ctxErr := b.ctx.Err(); ctxErr != nil {
		return 0, ctxErr
	}
	return n, err
}

func (b *cdnPlaybackContextBody) Close() error {
	b.once.Do(func() {
		b.closeErr = b.body.Close()
		if b.cancel != nil {
			b.cancel()
		}
		close(b.done)
	})
	return b.closeErr
}

// ValidateCDNPlaybackStream checks the provider's representation against the
// immutable imported media fingerprint and the exact range requested by the
// gateway. The caller must close Body on every validation failure.
//
// A complete response is exactly 200 with the full imported length. A partial
// response is exactly 206 with the requested inclusive interval and total
// length. HTTP 416 is generated by the gateway before provider access; a
// provider must never reinterpret an unsatisfiable client range.
func ValidateCDNPlaybackStream(binding CDNPlaybackBinding, requested *CDNPlaybackByteRange, stream CDNPlaybackStream) error {
	if binding.Validate() != nil || stream.Body == nil || stream.ContentLength <= 0 ||
		strings.ContainsAny(stream.ContentType, "\r\n") ||
		strings.ContainsAny(stream.ContentRange, "\r\n") {
		return ErrInvalidCDNPlaybackStream
	}
	mediaType, _, err := mime.ParseMediaType(stream.ContentType)
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "video/") {
		return ErrInvalidCDNPlaybackStream
	}
	if requested == nil {
		if stream.StatusCode != http.StatusOK || stream.ContentLength != binding.ContentLength || stream.ContentRange != "" {
			return ErrInvalidCDNPlaybackStream
		}
		return nil
	}
	if requested.Validate(binding.ContentLength) != nil {
		return ErrInvalidCDNPlaybackStream
	}
	expectedLength := requested.End - requested.Start + 1
	expectedRange := fmt.Sprintf("bytes %d-%d/%d", requested.Start, requested.End, binding.ContentLength)
	if stream.StatusCode != http.StatusPartialContent || stream.ContentLength != expectedLength || stream.ContentRange != expectedRange {
		return ErrInvalidCDNPlaybackStream
	}
	return nil
}
