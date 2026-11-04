package httpapi

import "net/http"

// maxRequestBodyBytes is a server-wide ceiling. Routes with tighter
// MaxBytesReader limits retain their own restrictions.
const maxRequestBodyBytes = 1 << 20

func limitRequestBodies(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"code": "REQUEST_TOO_LARGE"})
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
