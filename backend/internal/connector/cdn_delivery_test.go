package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSelectCDNPlaybackRange(t *testing.T) {
	tests := []struct {
		name          string
		values        []string
		contentLength int64
		status        int
		start         int64
		end           int64
		contentRange  string
		wantRange     bool
		wantError     bool
	}{
		{name: "full representation", contentLength: 1024, status: http.StatusOK},
		{name: "closed range", values: []string{"bytes=100-199"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 100, end: 199, contentRange: "bytes 100-199/1024", wantRange: true},
		{name: "open ended range", values: []string{"bytes=100-"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 100, end: 1023, contentRange: "bytes 100-1023/1024", wantRange: true},
		{name: "suffix range", values: []string{"bytes=-100"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 924, end: 1023, contentRange: "bytes 924-1023/1024", wantRange: true},
		{name: "suffix larger than representation", values: []string{"bytes=-2048"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 0, end: 1023, contentRange: "bytes 0-1023/1024", wantRange: true},
		{name: "end clamped to representation", values: []string{"bytes=1000-2048"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 1000, end: 1023, contentRange: "bytes 1000-1023/1024", wantRange: true},
		{name: "single byte at end", values: []string{"bytes=1023-1023"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 1023, end: 1023, contentRange: "bytes 1023-1023/1024", wantRange: true},
		{name: "start at EOF is unsatisfied", values: []string{"bytes=1024-"}, contentLength: 1024,
			status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */1024"},
		{name: "start beyond EOF is unsatisfied", values: []string{"bytes=2048-4096"}, contentLength: 1024,
			status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */1024"},
		{name: "duplicate Range fields", values: []string{"bytes=0-9", "bytes=10-19"}, contentLength: 1024, wantError: true},
		{name: "multi range", values: []string{"bytes=0-9,20-29"}, contentLength: 1024, wantError: true},
		{name: "wrong unit", values: []string{"items=0-9"}, contentLength: 1024, wantError: true},
		{name: "empty range", values: []string{"bytes="}, contentLength: 1024, wantError: true},
		{name: "reversed range", values: []string{"bytes=20-10"}, contentLength: 1024, wantError: true},
		{name: "zero suffix", values: []string{"bytes=-0"}, contentLength: 1024, wantError: true},
		{name: "signed start", values: []string{"bytes=+1-2"}, contentLength: 1024, wantError: true},
		{name: "whitespace", values: []string{"bytes=1 -2"}, contentLength: 1024, wantError: true},
		{name: "header injection", values: []string{"bytes=0-9\r\nX-Evil: 1"}, contentLength: 1024, wantError: true},
		{name: "start overflow", values: []string{"bytes=9223372036854775808-"}, contentLength: 1024, wantError: true},
		{name: "end overflow", values: []string{"bytes=0-9223372036854775808"}, contentLength: 1024, wantError: true},
		{name: "suffix overflow", values: []string{"bytes=-9223372036854775808"}, contentLength: 1024, wantError: true},
		{name: "unknown extra separator", values: []string{"bytes=1-2-3"}, contentLength: 1024, wantError: true},
		{name: "zero content length", contentLength: 0, wantError: true},
		{name: "negative content length", contentLength: -1, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SelectCDNPlaybackRange(test.values, test.contentLength)
			if test.wantError {
				if err == nil {
					t.Fatalf("invalid range selection accepted: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid range selection rejected: %v", err)
			}
			if got.StatusCode != test.status || got.ContentRange != test.contentRange {
				t.Fatalf("selection=%+v, want status=%d content-range=%q",
					got, test.status, test.contentRange)
			}
			if test.wantRange {
				if got.ByteRange == nil || got.ByteRange.Start != test.start || got.ByteRange.End != test.end {
					t.Fatalf("byte range=%+v, want %d-%d", got.ByteRange, test.start, test.end)
				}
			} else if got.ByteRange != nil {
				t.Fatalf("unexpected provider range for status %d: %+v", test.status, got.ByteRange)
			}
		})
	}
}

type blockingCDNPlaybackBody struct {
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newBlockingCDNPlaybackBody() *blockingCDNPlaybackBody {
	return &blockingCDNPlaybackBody{started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingCDNPlaybackBody) Read([]byte) (int, error) {
	b.startOnce.Do(func() { close(b.started) })
	<-b.release
	return 0, errors.New("provider body closed")
}

func (b *blockingCDNPlaybackBody) Close() error {
	b.stopOnce.Do(func() { close(b.release) })
	return nil
}

func TestOpenCDNPlaybackStreamClosesInFlightBodyWhenEntitlementContextIsCancelled(t *testing.T) {
	instance := activeCDNPlaybackInstance(t)
	principal := scopedPrincipal(t, instance.ExecuteScope)
	grant := validCDNPlaybackGrant(time.Now().UTC())
	body := newBlockingCDNPlaybackBody()
	provider := &fixedCDNPlaybackProvider{
		fakeCDNPlaybackProvider: &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
			kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
		}},
		stream: CDNPlaybackStream{
			StatusCode: http.StatusOK, ContentType: "video/mp4",
			ContentLength: grant.Binding.ContentLength, Body: body,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := OpenCDNPlaybackStream(ctx, instance, provider, principal, grant, nil, ExecutionPolicy{})
	if err != nil {
		cancel()
		t.Fatalf("open CDN stream: %v", err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, readErr := stream.Body.Read(make([]byte, 1))
		readDone <- readErr
	}()
	select {
	case <-body.started:
	case <-time.After(2 * time.Second):
		cancel()
		_ = stream.Body.Close()
		t.Fatal("provider read did not start")
	}

	// The HTTP Entitlement watcher cancels this context after the canonical
	// PostgreSQL revocation is observed. The executor must close the already
	// opened provider body rather than relying on the provider to poll ctx.
	cancel()
	select {
	case readErr := <-readDone:
		if !errors.Is(readErr, context.Canceled) {
			t.Fatalf("in-flight provider read returned %v, want context cancellation", readErr)
		}
	case <-time.After(2 * time.Second):
		_ = stream.Body.Close()
		t.Fatal("provider body remained blocked after entitlement context cancellation")
	}
	select {
	case <-body.release:
	default:
		t.Fatal("provider body was not closed after context cancellation")
	}
	_ = stream.Body.Close()
}

func TestCDNPlaybackBoundedBodyNeverLeaksExtraProviderBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := bindCDNPlaybackBody(ctx, io.NopCloser(strings.NewReader("validEXTRA")), 5)
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read bounded CDN body: %v", err)
	}
	if string(got) != "valid" {
		t.Fatalf("CDN provider overrun leaked: %q", got)
	}
}
