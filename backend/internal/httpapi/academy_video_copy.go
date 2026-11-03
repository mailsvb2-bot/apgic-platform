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
	for {
		n, readErr := src.Read(buffer)
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
			if flusher, ok := dst.(interface{ Flush() }); ok {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}
