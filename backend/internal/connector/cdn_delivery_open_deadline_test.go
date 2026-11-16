package connector

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A provider that honors the passed context must not be allowed to hold
// OpenPlaybackStream beyond the playback grant's deadline.
type deadlineBlockedCDNProvider struct {
	*fakeCDNPlaybackProvider
}

func (p *deadlineBlockedCDNProvider) OpenPlaybackStream(ctx context.Context, _ CDNPlaybackGrant, _ *CDNPlaybackByteRange) (CDNPlaybackStream, error) {
	p.streamCalls++
	<-ctx.Done()
	return CDNPlaybackStream{}, ctx.Err()
}

func TestCDNPlaybackProviderOpenBoundedByGrantDeadline(t *testing.T) {
	instance := activeCDNPlaybackInstance(t)
	provider := &deadlineBlockedCDNProvider{fakeCDNPlaybackProvider: &fakeCDNPlaybackProvider{
		fakeProvider: fakeProvider{kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN}},
	}}
	principal := scopedPrincipal(t, instance.ExecuteScope)
	grant := validCDNPlaybackGrant(time.Now().UTC().Add(-time.Second))
	grant.ExpiresAt = time.Now().UTC().Add(150 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := OpenCDNPlaybackStream(ctx, instance, provider, principal, grant, nil, ExecutionPolicy{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider open error = %v, want grant deadline exceeded", err)
	}
	if provider.streamCalls != 1 || stream.Body != nil {
		t.Fatalf("expired open must not deliver media: calls=%d stream=%+v", provider.streamCalls, stream)
	}
	if ctx.Err() != nil {
		t.Fatalf("caller context expired instead of grant deadline: %v", ctx.Err())
	}
}
