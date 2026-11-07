package httpapi

import (
	"context"
	"io"
	"time"
)

// academyVideoIdleReader limits one stalled upstream body read, not the
// lifetime of an authorized video. Successful reads reset the idle budget.
// The cancel function belongs to the existing gateway request and closes the
// upstream socket when a provider stops delivering bytes.
type academyVideoIdleReader struct {
	source  io.Reader
	cancel  context.CancelFunc
	timeout time.Duration
}

func (r academyVideoIdleReader) Read(p []byte) (int, error) {
	if r.source == nil || r.cancel == nil || r.timeout <= 0 {
		return 0, context.DeadlineExceeded
	}
	timer := time.AfterFunc(r.timeout, r.cancel)
	n, err := r.source.Read(p)
	// If the deadline fired, do not forward bytes from a read racing with
	// cancellation. The stream context is also checked before every write.
	if !timer.Stop() {
		r.cancel()
		return 0, context.DeadlineExceeded
	}
	return n, err
}
