package connector

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// A provider may return a syntactically valid stream after the playback grant
// expires. Such a body must be closed before the gateway can receive it.
type delayedCDNPlaybackProvider struct {
	*fakeCDNPlaybackProvider
	body *trackedCDNPlaybackBody
}

func (p *delayedCDNPlaybackProvider) OpenPlaybackStream(_ context.Context, grant CDNPlaybackGrant, _ *CDNPlaybackByteRange) (CDNPlaybackStream, error) {
	p.streamCalls++
	time.Sleep(time.Until(grant.ExpiresAt) + 20*time.Millisecond)
	p.body = &trackedCDNPlaybackBody{Reader: strings.NewReader(strings.Repeat("x", int(grant.Binding.ContentLength)))}
	return CDNPlaybackStream{
		StatusCode: 200, ContentType: "video/mp4",
		ContentLength: grant.Binding.ContentLength, Body: p.body,
	}, nil
}

func TestCDNPlaybackStreamRejectsGrantExpiredDuringProviderOpen(t *testing.T) {
	instance := activeCDNPlaybackInstance(t)
	provider := &delayedCDNPlaybackProvider{fakeCDNPlaybackProvider: &fakeCDNPlaybackProvider{
		fakeProvider: fakeProvider{kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN}},
	}}
	principal := scopedPrincipal(t, instance.ExecuteScope)
	grant := validCDNPlaybackGrant(time.Now().UTC().Add(-time.Second))
	grant.ExpiresAt = time.Now().UTC().Add(100 * time.Millisecond)
	stream, err := OpenCDNPlaybackStream(context.Background(), instance, provider, principal, grant, nil, ExecutionPolicy{})
	if !errors.Is(err, ErrInvalidCDNPlaybackGrant) {
		t.Fatalf("expired during open: got stream=%+v error=%v", stream, err)
	}
	if provider.streamCalls != 1 || provider.body == nil || !provider.body.closed {
		t.Fatalf("expired provider stream was not closed: calls=%d body=%v", provider.streamCalls, provider.body)
	}
	if stream.Body != nil {
		_ = stream.Body.Close()
		t.Fatal("expired provider body escaped to gateway")
	}
}

// Ensure the test's provider satisfies the same io.ReadCloser contract as
// production CDN adapters; no second media/provider owner is introduced.
var _ io.ReadCloser = (*trackedCDNPlaybackBody)(nil)
