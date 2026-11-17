package connector

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCDNPlaybackBoundedBodyRejectsTruncatedProviderResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := bindCDNPlaybackBody(ctx, io.NopCloser(strings.NewReader("short")), 10)
	defer body.Close()
	got, err := io.ReadAll(body)
	if string(got) != "short" {
		t.Fatalf("unexpected truncated response payload: %q", got)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated provider response error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestCDNPlaybackBoundedBodyExactLengthCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := bindCDNPlaybackBody(ctx, io.NopCloser(strings.NewReader("exact")), 5)
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "exact" {
		t.Fatalf("exact provider response = %q, %v", got, err)
	}
}

type cdnZeroProgressReader struct{}

func (cdnZeroProgressReader) Read([]byte) (int, error) { return 0, nil }

func TestCDNPlaybackBoundedBodyRejectsStalledProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := bindCDNPlaybackBody(ctx, io.NopCloser(cdnZeroProgressReader{}), 5)
	defer body.Close()
	_, err := io.ReadAll(body)
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("stalled provider error = %v, want io.ErrNoProgress", err)
	}
}

type cdnIntermittentReader struct {
	calls int
}

func (r *cdnIntermittentReader) Read(p []byte) (int, error) {
	r.calls++
	if r.calls%2 != 0 {
		return 0, nil
	}
	p[0] = 'x'
	return 1, nil
}

func TestCDNPlaybackBoundedBodyResetsNoProgressAfterBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := bindCDNPlaybackBody(ctx, io.NopCloser(&cdnIntermittentReader{}), 3)
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "xxx" {
		t.Fatalf("intermittent provider response = %q, %v", got, err)
	}
}

type cdnCloseTrackingStall struct {
	closed bool
}

func (r *cdnCloseTrackingStall) Read([]byte) (int, error) { return 0, nil }
func (r *cdnCloseTrackingStall) Close() error {
	r.closed = true
	return nil
}

func TestCDNPlaybackBoundedBodyClosesStalledUpstreamImmediately(t *testing.T) {
	upstream := &cdnCloseTrackingStall{}
	body := &cdnPlaybackBoundedBody{
		reader: &io.LimitedReader{R: upstream, N: 5},
		closer: upstream,
	}
	buffer := make([]byte, 5)
	for i := 0; i < 99; i++ {
		n, err := body.Read(buffer)
		if n != 0 || err != nil || upstream.closed {
			t.Fatalf("premature stall failure at read %d: n=%d err=%v closed=%v", i+1, n, err, upstream.closed)
		}
	}
	n, err := body.Read(buffer)
	if n != 0 || !errors.Is(err, io.ErrNoProgress) || !upstream.closed {
		t.Fatalf("stall must close upstream: n=%d err=%v closed=%v", n, err, upstream.closed)
	}
}
