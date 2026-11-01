package httpapi

import (
	"errors"
	"io"
	"time"
)

var errAcademyVideoRevoked = errors.New("academy media access revoked")

// academyCopyVideoAuthorized never writes a freshly read chunk once the
// canonical entitlement checker denies it. Rechecks occur by bytes or time.
// The independent watcher cancels blocked upstream reads.
func academyCopyVideoAuthorized(dst io.Writer, src io.Reader, authorized func() bool, maxBytes int, interval time.Duration) error {
	if maxBytes <= 0 || interval <= 0 {
		return errors.New("invalid media authorization interval")
	}
	buffer := make([]byte, 64<<10)
	sinceCheck := 0
	lastCheck := time.Now()
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			sinceCheck += n
			if sinceCheck >= maxBytes || time.Since(lastCheck) >= interval {
				if !authorized() {
					return errAcademyVideoRevoked
				}
				sinceCheck = 0
				lastCheck = time.Now()
			}
			written, writeErr := dst.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
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
