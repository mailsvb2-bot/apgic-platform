package httpapi

import (
	"errors"
	"io"
)

var errAcademyVideoRevoked = errors.New("academy media access revoked")

// academyCopyVideoAuthorized rechecks canonical access before EVERY 64 KiB
// write, including the first. A revoked enrollment must not receive the
// remainder of a previously authorized chunk. The independent watcher in
// the HTTP handler cancels stalled upstream reads between chunks.
func academyCopyVideoAuthorized(dst io.Writer, src io.Reader, authorized func() bool) error {
	buffer := make([]byte, 64<<10)
	// Do not read even the first provider chunk after access has been revoked.
	// The HTTP handler also checks access before starting the upstream request,
	// but the decision can change before the byte copier starts.
	if !authorized() {
		return errAcademyVideoRevoked
	}
	// A broken upstream io.Reader may repeatedly return (0, nil). Without a
	// bound this loops forever, consumes CPU and never rechecks entitlement.
	// Fail closed instead of leaving a zombie media stream running.
	emptyReads := 0
	for {
		n, readErr := src.Read(buffer)
		if n == 0 && readErr == nil {
			if !authorized() {
				return errAcademyVideoRevoked
			}
			emptyReads++
			if emptyReads >= 100 {
				return io.ErrNoProgress
			}
			continue
		}
		emptyReads = 0
		if n > 0 {
			if !authorized() {
				return errAcademyVideoRevoked
			}
			written, writeErr := dst.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			// net/http may retain the last bytes of a live response in its
			// write buffer until the upstream finishes. A paused cloud source
			// must not prevent an already-authorized first video chunk from
			// reaching the learner. Flush only after a successful write.
			// Prefer error-aware flushing: a revoked, disconnected or
			// stalled learner can make net/http FlushError fail even after
			// Write accepted the bytes into its internal buffer. Do not
			// continue reading the provider after such a failure.
			if flusher, ok := dst.(interface{ FlushError() error }); ok {
				if !authorized() {
					return errAcademyVideoRevoked
				}
				if err := flusher.FlushError(); err != nil {
					return err
				}
				// Authorization can change WHILE FlushError blocks.
				if !authorized() {
					return errAcademyVideoRevoked
				}
			} else if flusher, ok := dst.(interface{ Flush() }); ok {
				if !authorized() {
					return errAcademyVideoRevoked
				}
				flusher.Flush()
				if !authorized() {
					return errAcademyVideoRevoked
				}
			} else if !authorized() {
				// A non-HTTP writer has no Flush boundary. Revocation during
				// Write must still stop BEFORE reading another provider chunk.
				return errAcademyVideoRevoked
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				// A provider may return EOF immediately after a canonical
				// entitlement revocation. Do not report successful delivery
				// without checking the final access boundary.
				if !authorized() {
					return errAcademyVideoRevoked
				}
				return nil
			}
			return readErr
		}
	}
}
