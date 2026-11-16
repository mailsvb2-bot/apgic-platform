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
