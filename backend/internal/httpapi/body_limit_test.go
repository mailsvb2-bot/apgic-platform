package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimitRequestBodiesCapsUnboundedDecoders(t *testing.T) {
	var readErr error
	h := limitRequestBodies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}), 16)
	req := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 64)))
	req.ContentLength = -1
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil {
		t.Fatal("chunked body beyond the ceiling must fail")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 64))))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("declared oversize body: %d", w.Code)
	}
	readErr = nil
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("ok")))
	if readErr != nil {
		t.Fatal(readErr)
	}
}
