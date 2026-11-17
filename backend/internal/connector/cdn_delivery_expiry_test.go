package connector

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// A playback grant cannot keep an already-open stream alive after its expiry,
// even if the caller's entitlement watcher has not yet cancelled its context.
func TestCDNPlaybackBodyExpiresDuringActiveRead(t *testing.T) {
	upstream := &cdnCloseTrackingStall{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := bindCDNPlaybackBody(ctx, upstream, 10, time.Now().Add(-time.Second))
	defer body.Close()
	buffer := make([]byte, 1)
	n, err := body.Read(buffer)
	if n != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired stream read = (%d, %v), want deadline exceeded", n, err)
	}
	select {
	case <-body.(*cdnPlaybackContextBody).done:
	case <-time.After(time.Second):
		t.Fatal("expired stream did not close upstream")
	}
	if !upstream.closed {
		t.Fatal("expired stream left provider body open")
	}
}

func TestCDNPlaybackBodyStillReadsBeforeGrantExpiry(t *testing.T) {
	body := bindCDNPlaybackBody(context.Background(), io.NopCloser(strings.NewReader("ok")), 2, time.Now().Add(time.Minute))
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "ok" {
		t.Fatalf("unexpired stream = %q, %v", got, err)
	}
}

func TestCDNPlaybackBodyCloseCancelsProviderOpenContext(t *testing.T) {
	providerCtx, cancelOpen := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	upstream := &cdnCloseTrackingStall{}
	body := bindCDNPlaybackBodyWithCancel(providerCtx, upstream, 10, cancelOpen)
	if err := providerCtx.Err(); err != nil {
		t.Fatalf("provider context was cancelled before streaming: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("closing playback stream: %v", err)
	}
	select {
	case <-providerCtx.Done():
		if !errors.Is(providerCtx.Err(), context.Canceled) {
			t.Fatalf("provider context = %v, want cancelled", providerCtx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("closing the stream did not release the provider deadline context")
	}
	if !upstream.closed {
		t.Fatal("closing the stream did not close the upstream provider body")
	}
}
