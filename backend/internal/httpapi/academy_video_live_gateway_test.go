package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// This opt-in test uses an actual author-owned Yandex video and the hardened
// gateway transport. A skipped test is NOT proof of live OAuth or playback.
// It does not replace the browser/PSP/Entitlement staging release gate.
func TestLiveAcademyYandexGatewaySeekAndRevocation(t *testing.T) {
	token := os.Getenv("APGIC_TEST_YANDEX_OAUTH_TOKEN")
	path := os.Getenv("APGIC_TEST_YANDEX_VIDEO_PATH")
	if token == "" || path == "" {
		t.Skip("real author OAuth token and Yandex test video are required")
	}
	if !academy.ValidYandexDiskPath(path) {
		t.Fatal("invalid live test video path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := academy.YandexDiskClient{}
	file, err := provider.VerifyVideo(ctx, token, path)
	if err != nil || file.Size < 2 {
		t.Fatal("real provider video verification failed")
	}
	href, err := provider.TemporaryDownload(ctx, token, path)
	if err != nil || !academy.ValidateYandexDownloadURL(href) {
		t.Fatal("real provider download URL unavailable or untrusted")
	}

	var allowed atomic.Bool
	allowed.Store(true)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if !allowed.Load() {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		requested, ok := academyVideoRequestedRange(r.Header)
		if !ok || requested == "" {
			http.Error(w, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		upstreamRequest, err := academyVideoUpstreamRequest(r.Context(), href, requested)
		if err != nil {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		// The outbound request must never contain a learner cookie or the
		// author's OAuth token. Only the signed temporary URL is used.
		if upstreamRequest.Header.Get("Authorization") != "" || upstreamRequest.Header.Get("Cookie") != "" {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		upstream, err := academyVideoUpstreamClient().Do(upstreamRequest)
		if err != nil {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		defer upstream.Body.Close()
		if upstream.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			if !academyVideoUnsatisfiedRangeHeadersAllowed(requested, upstream.Header) ||
				!academyVideoRepresentationSizeMatches(upstream.StatusCode, upstream.Header, file.Size) {
				http.Error(w, "Unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Range", upstream.Header.Get("Content-Range"))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if !academyVideoUpstreamResponseAllowed(requested, upstream.StatusCode, upstream.Header) ||
			!academyVideoRepresentationSizeMatches(upstream.StatusCode, upstream.Header, file.Size) {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := academyVideoBodyForLearner(upstream.StatusCode, upstream.Header, upstream.Body)
		if err != nil {
			http.Error(w, "Unavailable", http.StatusServiceUnavailable)
			return
		}
		length, err := academyVideoPartialLength(upstream.Header.Get("Content-Range"))
		if err != nil || !allowed.Load() {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", upstream.Header.Get("Content-Type"))
		w.Header().Set("Content-Range", upstream.Header.Get("Content-Range"))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_ = academyCopyVideoAuthorized(w, body, allowed.Load)
	}))
	defer gateway.Close()

	client := gateway.Client()
	client.Timeout = 30 * time.Second
	end := file.Size - 1
	if end > 1023 {
		end = 1023
	}
	seek := func(rangeHeader string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, gateway.URL, nil)
		if err != nil {
			t.Fatal("cannot construct learner request")
		}
		req.Header.Set("Range", rangeHeader)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("learner gateway request failed")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if err != nil {
			t.Fatal("learner gateway response was incomplete")
		}
		return resp, body
	}

	resp, body := seek(fmt.Sprintf("bytes=0-%d", end))
	if resp.StatusCode != http.StatusPartialContent || int64(len(body)) != end+1 ||
		!strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes 0-") {
		t.Fatalf("real cloud seek failed: HTTP %d, received %d bytes", resp.StatusCode, len(body))
	}
	if resp.Header.Get("Location") != "" || bytes.Contains(body, []byte(token)) ||
		bytes.Contains(body, []byte(href)) {
		t.Fatal("provider credential or URL escaped the gateway")
	}

	// A new seek after revocation must not make another provider request,
	// and must not return even previously buffered media bytes.
	allowed.Store(false)
	resp, body = seek(fmt.Sprintf("bytes=0-%d", end))
	if resp.StatusCode != http.StatusForbidden || bytes.Contains(body, []byte(token)) ||
		bytes.Contains(body, []byte(href)) || resp.Header.Get("Location") != "" {
		t.Fatal("revoked learner received cloud media or provider secrets")
	}
}

// The same real upstream response is also passed through the production
// authorized copier. Revocation after its first write must stop the next
// upstream read, not merely close a simulated manifest.
type academyLiveRevokingWriter struct {
	bytes.Buffer
	allowed *atomic.Bool
	writes  int
}

func (w *academyLiveRevokingWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.writes++
	w.allowed.Store(false)
	return n, err
}

func TestLiveAcademyYandexGatewayStopsInFlightAfterRevocation(t *testing.T) {
	token := os.Getenv("APGIC_TEST_YANDEX_OAUTH_TOKEN")
	path := os.Getenv("APGIC_TEST_YANDEX_VIDEO_PATH")
	if token == "" || path == "" {
		t.Skip("real author OAuth token and Yandex test video are required")
	}
	if !academy.ValidYandexDiskPath(path) {
		t.Fatal("invalid live test video path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	provider := academy.YandexDiskClient{}
	file, err := provider.VerifyVideo(ctx, token, path)
	if err != nil {
		t.Fatal("real provider video verification failed")
	}
	if file.Size < 131072 {
		t.Skip("in-flight proof requires an author-owned video of at least 128 KiB")
	}
	href, err := provider.TemporaryDownload(ctx, token, path)
	if err != nil || !academy.ValidateYandexDownloadURL(href) {
		t.Fatal("real provider download URL unavailable or untrusted")
	}
	request, err := academyVideoUpstreamRequest(ctx, href, "bytes=0-131071")
	if err != nil {
		t.Fatal("invalid gateway range request")
	}
	response, err := academyVideoUpstreamClient().Do(request)
	if err != nil {
		t.Fatal("real cloud transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent ||
		!academyVideoUpstreamResponseAllowed("bytes=0-131071", response.StatusCode, response.Header) ||
		!academyVideoRepresentationSizeMatches(response.StatusCode, response.Header, file.Size) {
		t.Fatal("real provider returned an invalid partial representation")
	}
	body, err := academyVideoBodyForLearner(response.StatusCode, response.Header, response.Body)
	if err != nil {
		t.Fatal("invalid real provider byte range")
	}
	var allowed atomic.Bool
	allowed.Store(true)
	writer := &academyLiveRevokingWriter{allowed: &allowed}
	reader := &academyCountingVideoReader{reader: body}
	err = academyCopyVideoAuthorized(writer, reader, allowed.Load)
	if !errors.Is(err, errAcademyVideoRevoked) || writer.writes != 1 ||
		writer.Len() > 64<<10 || reader.reads != 1 {
		t.Fatalf("in-flight revocation failed: writes=%d bytes=%d reads=%d", writer.writes, writer.Len(), reader.reads)
	}
}
