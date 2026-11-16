package connector

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

const (
	cdnTestSubject          = "11111111-1111-4111-8111-111111111111"
	cdnTestEnrollment       = "22222222-2222-4222-8222-222222222222"
	cdnTestEntitlement      = "33333333-3333-4333-8333-333333333333"
	cdnTestCourse           = "44444444-4444-4444-8444-444444444444"
	cdnTestVersion          = "55555555-5555-4555-8555-555555555555"
	cdnTestLesson           = "66666666-6666-4666-8666-666666666666"
	cdnTestMedia            = "77777777-7777-4777-8777-777777777777"
	cdnTestSession          = "88888888-8888-4888-8888-888888888888"
	cdnTestProviderInstance = "99999999-9999-4999-8999-999999999999"
)

func validCDNPlaybackBinding() CDNPlaybackBinding {
	return CDNPlaybackBinding{
		SubjectID:       cdnTestSubject,
		EnrollmentID:    cdnTestEnrollment,
		EntitlementID:   cdnTestEntitlement,
		CourseID:        cdnTestCourse,
		CourseVersionID: cdnTestVersion,
		LessonID:        cdnTestLesson,
		MediaID:         cdnTestMedia,
		ContentSHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContentLength:   1024,
	}
}

func validCDNPlaybackGrant(now time.Time) CDNPlaybackGrant {
	return CDNPlaybackGrant{
		ContractVersion:    CDNPlaybackContractVersion,
		ProviderKind:       "provider-a",
		ProviderInstanceID: cdnTestProviderInstance,
		SessionID:          cdnTestSession,
		Binding:            validCDNPlaybackBinding(),
		IssuedAt:           now.UTC(),
		ExpiresAt:          now.UTC().Add(10 * time.Minute),
	}
}

func signedCDNPlaybackAttestation(t *testing.T, grant CDNPlaybackGrant, privateKey ed25519.PrivateKey, keyID string) CDNPlaybackAttestation {
	t.Helper()
	attestation := CDNPlaybackAttestation{
		ContractVersion:    CDNPlaybackContractVersion,
		ProviderKind:       grant.ProviderKind,
		ProviderInstanceID: grant.ProviderInstanceID,
		ProviderEventID:    "event-1",
		SessionID:          grant.SessionID,
		Binding:            grant.Binding,
		DurationMillis:     100000,
		WatchedRanges: []CDNPlaybackRange{
			{StartMillis: 0, EndMillis: 45000},
			{StartMillis: 50000, EndMillis: 95000},
		},
		StartedAt:   grant.IssuedAt.Add(time.Second),
		CompletedAt: grant.IssuedAt.Add(2 * time.Minute),
		KeyID:       keyID,
	}
	resignCDNPlaybackAttestation(t, &attestation, privateKey)
	return attestation
}

func resignCDNPlaybackAttestation(t *testing.T, attestation *CDNPlaybackAttestation, privateKey ed25519.PrivateKey) {
	t.Helper()
	message, err := attestation.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message))
}

func TestCDNPlaybackAttestationRequiresExactGrantBindingAndTrustedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 22, 0, 0, 0, time.UTC)
	grant := validCDNPlaybackGrant(now)
	attestation := signedCDNPlaybackAttestation(t, grant, privateKey, "cdn-key-1")

	verified, err := VerifyCDNPlaybackAttestation(grant, attestation, "cdn-key-1", publicKey, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Valid() || verified.Binding() != grant.Binding || verified.SessionID() != grant.SessionID ||
		verified.ProviderEventID() != attestation.ProviderEventID ||
		verified.WatchedMilliseconds() != 90000 || !verified.CompletedAt().Equal(attestation.CompletedAt) {
		t.Fatalf("verified evidence does not preserve the signed binding: %#v", verified)
	}
	if verified.CoversEntireDuration() || verified.CompletedSeconds() != 0 {
		t.Fatal("partial playback ranges were accepted as a lesson completion")
	}
	full := attestation
	full.WatchedRanges = []CDNPlaybackRange{{StartMillis: 0, EndMillis: full.DurationMillis}}
	resignCDNPlaybackAttestation(t, &full, privateKey)
	complete, err := VerifyCDNPlaybackAttestation(grant, full, "cdn-key-1", publicKey, now.Add(3*time.Minute))
	if err != nil || !complete.CoversEntireDuration() || complete.CompletedSeconds() != 100 {
		t.Fatalf("full timeline evidence was not recognized: %+v %v", complete, err)
	}
	ref, err := complete.EvidenceReference()
	if err != nil || !strings.HasPrefix(ref, "sha256:") || len(ref) != len("sha256:")+64 {
		t.Fatalf("signed evidence did not produce a stable digest reference: %q %v", ref, err)
	}
}

func TestCDNPlaybackAttestationRejectsTamperedBindingAndSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 22, 0, 0, 0, time.UTC)
	grant := validCDNPlaybackGrant(now)
	base := signedCDNPlaybackAttestation(t, grant, privateKey, "cdn-key-1")

	cases := map[string]func(*CDNPlaybackAttestation){
		"foreign learner": func(a *CDNPlaybackAttestation) { a.Binding.SubjectID = "99999999-9999-4999-8999-999999999999" },
		"foreign lesson":  func(a *CDNPlaybackAttestation) { a.Binding.LessonID = "99999999-9999-4999-8999-999999999999" },
		"changed content": func(a *CDNPlaybackAttestation) {
			a.Binding.ContentSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"changed entitlement":     func(a *CDNPlaybackAttestation) { a.Binding.EntitlementID = "99999999-9999-4999-8999-999999999999" },
		"wrong key id":            func(a *CDNPlaybackAttestation) { a.KeyID = "untrusted-key" },
		"changed ranges":          func(a *CDNPlaybackAttestation) { a.WatchedRanges[0].EndMillis-- },
		"wrong session":           func(a *CDNPlaybackAttestation) { a.SessionID = "99999999-9999-4999-8999-999999999999" },
		"wrong provider instance": func(a *CDNPlaybackAttestation) { a.ProviderInstanceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" },
		"wrong provider event":    func(a *CDNPlaybackAttestation) { a.ProviderEventID = "event-for-another-session" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			attestation := base
			attestation.WatchedRanges = append([]CDNPlaybackRange(nil), base.WatchedRanges...)
			mutate(&attestation)
			if _, err := VerifyCDNPlaybackAttestation(grant, attestation, "cdn-key-1", publicKey, now.Add(3*time.Minute)); err == nil {
				t.Fatal("tampered provider evidence was accepted")
			}
		})
	}
}

func TestCDNPlaybackAttestationRejectsVerificationAfterGrantExpiry(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 22, 0, 0, 0, time.UTC)
	grant := validCDNPlaybackGrant(now)
	attestation := signedCDNPlaybackAttestation(t, grant, privateKey, "cdn-key-1")
	if _, err := VerifyCDNPlaybackAttestation(grant, attestation, "cdn-key-1", publicKey, grant.ExpiresAt.Add(time.Minute)); err == nil {
		t.Fatal("provider attestation was accepted after the grant's bounded lifetime")
	}
}

func TestCDNPlaybackAttestationRejectsInvalidRangesAndGrantWindow(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 22, 0, 0, 0, time.UTC)
	grant := validCDNPlaybackGrant(now)
	cases := map[string]func(*CDNPlaybackAttestation, *CDNPlaybackGrant){
		"overlapping ranges": func(a *CDNPlaybackAttestation, _ *CDNPlaybackGrant) {
			a.WatchedRanges[1].StartMillis = 40000
		},
		"range beyond duration": func(a *CDNPlaybackAttestation, _ *CDNPlaybackGrant) {
			a.WatchedRanges[1].EndMillis = 100001
		},
		"empty range": func(a *CDNPlaybackAttestation, _ *CDNPlaybackGrant) {
			a.WatchedRanges[0].EndMillis = 0
		},
		"started before grant": func(a *CDNPlaybackAttestation, _ *CDNPlaybackGrant) {
			a.StartedAt = now.Add(-time.Second)
		},
		"completed after grant": func(a *CDNPlaybackAttestation, g *CDNPlaybackGrant) {
			a.CompletedAt = g.ExpiresAt.Add(time.Second)
		},
		"future completion": func(a *CDNPlaybackAttestation, _ *CDNPlaybackGrant) {
			a.CompletedAt = now.Add(10 * time.Minute)
		},
		"grant too long": func(_ *CDNPlaybackAttestation, g *CDNPlaybackGrant) {
			g.ExpiresAt = g.IssuedAt.Add(CDNPlaybackMaxGrantLifetime + time.Second)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			g := grant
			a := signedCDNPlaybackAttestation(t, g, privateKey, "cdn-key-1")
			mutate(&a, &g)
			resignCDNPlaybackAttestation(t, &a, privateKey)
			if _, err := VerifyCDNPlaybackAttestation(g, a, "cdn-key-1", publicKey, now.Add(3*time.Minute)); err == nil {
				t.Fatal("invalid provider evidence was accepted")
			}
		})
	}
}

type fakeCDNPlaybackProvider struct {
	fakeProvider
	grantCalls  int
	streamCalls int
	revokeCalls int
	lastRevoked CDNPlaybackGrant
}

func (p *fakeCDNPlaybackProvider) IssuePlaybackGrant(_ context.Context, request CDNPlaybackRequest) (CDNPlaybackGrant, error) {
	p.grantCalls++
	if err := request.Validate(); err != nil {
		return CDNPlaybackGrant{}, err
	}
	return validCDNPlaybackGrant(time.Now().UTC()), nil
}

func (p *fakeCDNPlaybackProvider) RevokePlaybackGrant(_ context.Context, grant CDNPlaybackGrant) error {
	p.revokeCalls++
	p.lastRevoked = grant
	return nil
}

func (p *fakeCDNPlaybackProvider) VerifyPlaybackAttestation(_ context.Context, _ CDNPlaybackGrant, _ CDNPlaybackAttestation) (VerifiedCDNPlaybackAttestation, error) {
	return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
}

func (p *fakeCDNPlaybackProvider) OpenPlaybackStream(_ context.Context, grant CDNPlaybackGrant, requested *CDNPlaybackByteRange) (CDNPlaybackStream, error) {
	p.streamCalls++
	if err := grant.Validate(); err != nil {
		return CDNPlaybackStream{}, err
	}
	status := 200
	length := grant.Binding.ContentLength
	contentRange := ""
	if requested != nil {
		if err := requested.Validate(length); err != nil {
			return CDNPlaybackStream{}, err
		}
		status = 206
		length = requested.End - requested.Start + 1
		contentRange = fmt.Sprintf("bytes %d-%d/%d", requested.Start, requested.End, grant.Binding.ContentLength)
	}
	return CDNPlaybackStream{
		StatusCode: status, ContentType: "video/mp4", ContentLength: length,
		ContentRange: contentRange, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", int(length)))),
	}, nil
}

type controlOnlyCDNPlaybackProvider struct{ fakeProvider }

func (p *controlOnlyCDNPlaybackProvider) IssuePlaybackGrant(_ context.Context, request CDNPlaybackRequest) (CDNPlaybackGrant, error) {
	if err := request.Validate(); err != nil {
		return CDNPlaybackGrant{}, err
	}
	return validCDNPlaybackGrant(time.Now().UTC()), nil
}
func (p *controlOnlyCDNPlaybackProvider) RevokePlaybackGrant(context.Context, CDNPlaybackGrant) error {
	return nil
}
func (p *controlOnlyCDNPlaybackProvider) VerifyPlaybackAttestation(context.Context, CDNPlaybackGrant, CDNPlaybackAttestation) (VerifiedCDNPlaybackAttestation, error) {
	return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
}

func TestCDNPlaybackCapabilityMustUseExistingCDNProviderClass(t *testing.T) {
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		t.Fatal(err)
	}
	controlOnly := &controlOnlyCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	if err := ValidateCDNPlaybackProvider(controlOnly); err == nil {
		t.Fatal("CDN provider without byte-delivery capability was accepted as production-ready")
	}
	ordinary := &fakeProvider{kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN}}
	if err := ValidateCDNPlaybackProvider(ordinary); err == nil {
		t.Fatal("generic CDN connector without versioned playback contract was accepted")
	}
	if err := ValidateCDNPlaybackProvider(nil); err == nil {
		t.Fatal("nil CDN provider was accepted")
	}
}

func TestCDNPlaybackStreamValidatesExactFullAndPartialRepresentations(t *testing.T) {
	binding := validCDNPlaybackBinding()
	newBody := func(length int64) io.ReadCloser {
	return io.NopCloser(strings.NewReader(strings.Repeat("x", int(length))))
}
	full := CDNPlaybackStream{
		StatusCode: 200, ContentType: "video/mp4", ContentLength: binding.ContentLength,
		Body: newBody(binding.ContentLength),
	}
	if err := ValidateCDNPlaybackStream(binding, nil, full); err != nil {
		t.Fatalf("valid full representation rejected: %v", err)
	}
	_ = full.Body.Close()
	partialRange := &CDNPlaybackByteRange{Start: 100, End: 199}
	partial := CDNPlaybackStream{
		StatusCode: 206, ContentType: "video/mp4", ContentLength: 100,
		ContentRange: "bytes 100-199/1024", Body: newBody(100),
	}
	if err := ValidateCDNPlaybackStream(binding, partialRange, partial); err != nil {
		t.Fatalf("valid partial representation rejected: %v", err)
	}
	_ = partial.Body.Close()

	cases := []struct {
		name         string
		rangeRequest *CDNPlaybackByteRange
		stream       CDNPlaybackStream
	}{
		{name: "provider must not decide 416", rangeRequest: partialRange, stream: CDNPlaybackStream{StatusCode: 416, ContentRange: "bytes */1024", ContentLength: 0, ContentType: "video/mp4", Body: newBody(0)}},
		{name: "partial without range", stream: CDNPlaybackStream{StatusCode: 206, ContentType: "video/mp4", ContentLength: 100, ContentRange: "bytes 100-199/1024", Body: newBody(100)}},
		{name: "full response for range", rangeRequest: partialRange, stream: CDNPlaybackStream{StatusCode: 200, ContentType: "video/mp4", ContentLength: binding.ContentLength, Body: newBody(binding.ContentLength)}},
		{name: "wrong content range", rangeRequest: partialRange, stream: CDNPlaybackStream{StatusCode: 206, ContentType: "video/mp4", ContentLength: 100, ContentRange: "bytes 99-198/1024", Body: newBody(100)}},
		{name: "wrong partial length", rangeRequest: partialRange, stream: CDNPlaybackStream{StatusCode: 206, ContentType: "video/mp4", ContentLength: 99, ContentRange: "bytes 100-199/1024", Body: newBody(99)}},
		{name: "non-video content", stream: CDNPlaybackStream{StatusCode: 200, ContentType: "text/html", ContentLength: binding.ContentLength, Body: newBody(binding.ContentLength)}},
		{name: "header injection", stream: CDNPlaybackStream{StatusCode: 200, ContentType: "video/mp4\r\nLocation: https://origin.invalid", ContentLength: binding.ContentLength, Body: newBody(binding.ContentLength)}},
		{name: "missing body", stream: CDNPlaybackStream{StatusCode: 200, ContentType: "video/mp4", ContentLength: binding.ContentLength}},
		{name: "unsatisfiable range", rangeRequest: &CDNPlaybackByteRange{Start: 1024, End: 1024}, stream: CDNPlaybackStream{StatusCode: 206, ContentType: "video/mp4", ContentLength: 1, ContentRange: "bytes 1024-1024/1024", Body: newBody(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCDNPlaybackStream(binding, tc.rangeRequest, tc.stream); err == nil {
				t.Fatal("invalid provider stream metadata was accepted")
			}
			if tc.stream.Body != nil {
				_ = tc.stream.Body.Close()
			}
		})
	}
}

func TestCDNPlaybackProviderRangeContractDoesNotExposeOriginFields(t *testing.T) {
	// The provider result is deliberately limited to a stream body and a
	// small allowlisted metadata tuple. It has no URL, token, cookie or
	// arbitrary-header field that could reach learner JSON.
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	grant := validCDNPlaybackGrant(time.Now().UTC())
	requested := &CDNPlaybackByteRange{Start: 0, End: 127}
	stream, err := provider.OpenPlaybackStream(context.Background(), grant, requested)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if err := ValidateCDNPlaybackStream(grant.Binding, requested, stream); err != nil {
		t.Fatal(err)
	}
	if provider.streamCalls != 1 {
		t.Fatalf("stream calls=%d, want 1", provider.streamCalls)
	}
}

func TestCDNPlaybackRequestValidatesHierarchicalIdempotencyKey(t *testing.T) {
	request := CDNPlaybackRequest{ProviderInstanceID: cdnTestProviderInstance, Binding: validCDNPlaybackBinding()}
	for _, key := range []string{
		"academy/playback/attempt-1",
		"attempt-1",
	} {
		request.IdempotencyKey = key
		if err := request.Validate(); err != nil {
			t.Fatalf("valid idempotency key %q rejected: %v", key, err)
		}
	}
	for _, key := range []string{
		"",
		"/academy/attempt-1",
		"academy//attempt-1",
		"academy/../attempt-1",
		"academy/./attempt-1",
		"academy/attempt-1/",
		"academy/attempt-1\n",
		strings.Repeat("a", 129),
	} {
		request.IdempotencyKey = key
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid idempotency key %q accepted", key)
		}
	}
}

func TestCDNPlaybackRequestRejectsInvalidBinding(t *testing.T) {
	request := CDNPlaybackRequest{
		IdempotencyKey:     "academy/playback/attempt-1",
		ProviderInstanceID: cdnTestProviderInstance,
		Binding:            validCDNPlaybackBinding(),
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Binding.ContentSHA256 = "not-a-digest"
	if err := request.Validate(); err == nil {
		t.Fatal("invalid media digest was accepted")
	}
	request.Binding = validCDNPlaybackBinding()
	request.ProviderInstanceID = "not-a-uuid"
	if request.Validate() == nil {
		t.Fatal("invalid provider instance ID was accepted")
	}
}

type verifyingCDNPlaybackProvider struct {
	fakeCDNPlaybackProvider
	keyID     string
	publicKey ed25519.PublicKey
	now       time.Time
}

func (p *verifyingCDNPlaybackProvider) VerifyPlaybackAttestation(
	_ context.Context,
	grant CDNPlaybackGrant,
	attestation CDNPlaybackAttestation,
) (VerifiedCDNPlaybackAttestation, error) {
	return VerifyCDNPlaybackAttestation(grant, attestation, p.keyID, p.publicKey, p.now)
}

func activeCDNPlaybackInstance(t *testing.T) Instance {
	t.Helper()
	instance, err := NewInstance(cdnTestProviderInstance, CapabilityCDN, "provider-a", "config/cdn")
	if err != nil {
		t.Fatal(err)
	}
	instance.Status = StatusActive
	return instance
}

func TestTypedCDNPlaybackGrantUsesCanonicalConnectorAuthorization(t *testing.T) {
	instance := activeCDNPlaybackInstance(t)
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	request := CDNPlaybackRequest{
		IdempotencyKey:     "academy/playback/attempt-1",
		ProviderInstanceID: cdnTestProviderInstance,
		Binding:            validCDNPlaybackBinding(),
	}

	wrongPrincipal := scopedPrincipal(t, "connector:execute:PAYMENT_PROVIDER")
	if _, err := IssueCDNPlaybackGrant(context.Background(), instance, provider, wrongPrincipal, request, ExecutionPolicy{}); err != ErrConnectorScopeDenied {
		t.Fatalf("expected canonical scope denial, got %v", err)
	}
	if provider.grantCalls != 0 {
		t.Fatal("CDN provider was called before canonical connector authorization")
	}

	principal := scopedPrincipal(t, instance.ExecuteScope)
	grant, err := IssueCDNPlaybackGrant(context.Background(), instance, provider, principal, request, ExecutionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if grant.ProviderKind != instance.ProviderKind || grant.ProviderInstanceID != instance.ID ||
		grant.Binding != request.Binding || !grant.ExpiresAt.After(time.Now()) {
		t.Fatalf("provider returned an unbound or expired grant: %#v", grant)
	}
	if provider.grantCalls != 1 {
		t.Fatalf("expected one provider grant call, got %d", provider.grantCalls)
	}
}

func TestTypedCDNPlaybackRevocationUsesDedicatedScopeAndSurvivesDisable(t *testing.T) {
	instance := activeCDNPlaybackInstance(t)
	instance.Status = StatusDisabled
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	grant := validCDNPlaybackGrant(time.Now().UTC())
	grant.ExpiresAt = grant.IssuedAt.Add(time.Minute)
	policy := ExecutionPolicy{Now: func() time.Time { return grant.ExpiresAt.Add(time.Minute) }}

	wrongPrincipal := scopedPrincipal(t, instance.ExecuteScope)
	if err := RevokeCDNPlaybackGrant(context.Background(), instance, provider, wrongPrincipal, grant, policy); err != ErrConnectorScopeDenied {
		t.Fatalf("ordinary execute scope was allowed to revoke playback: %v", err)
	}
	if provider.revokeCalls != 0 {
		t.Fatal("provider was called before revoke scope authorization")
	}
	principal := scopedPrincipal(t, RevokeScopeFor(CapabilityCDN))
	if err := RevokeCDNPlaybackGrant(context.Background(), instance, provider, principal, grant, policy); err != nil {
		t.Fatalf("disabled connector could not revoke its active playback session: %v", err)
	}
	if provider.revokeCalls != 1 || provider.lastRevoked.SessionID != grant.SessionID {
		t.Fatalf("provider did not receive the exact idempotent revocation: calls=%d grant=%+v", provider.revokeCalls, provider.lastRevoked)
	}
}

func TestTypedCDNPlaybackRevocationCannotCrossConnectorInstances(t *testing.T) {
	first := activeCDNPlaybackInstance(t)
	second := first
	second.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	grant := validCDNPlaybackGrant(time.Now().UTC())
	principal := scopedPrincipal(t, RevokeScopeFor(CapabilityCDN))
	if err := RevokeCDNPlaybackGrant(context.Background(), second, provider, principal, grant, ExecutionPolicy{}); err != ErrInvalidCDNPlaybackGrant {
		t.Fatalf("revocation crossed connector instances: %v", err)
	}
	if provider.revokeCalls != 0 {
		t.Fatal("provider was called for a grant from another connector instance")
	}
}

func TestTypedCDNPlaybackGrantCannotCrossConnectorInstances(t *testing.T) {
	first := activeCDNPlaybackInstance(t)
	second := first
	second.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	request := CDNPlaybackRequest{
		IdempotencyKey:     "academy/playback/attempt-1",
		ProviderInstanceID: first.ID,
		Binding:            validCDNPlaybackBinding(),
	}
	principal := scopedPrincipal(t, first.ExecuteScope)
	if _, err := IssueCDNPlaybackGrant(context.Background(), second, provider, principal, request, ExecutionPolicy{}); err != ErrInvalidCDNPlaybackRequest {
		t.Fatalf("grant request crossed connector instances: %v", err)
	}
	if provider.grantCalls != 0 {
		t.Fatal("provider was called for a grant bound to another connector instance")
	}
	grant := validCDNPlaybackGrant(time.Now().UTC())
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attestation := signedCDNPlaybackAttestation(t, grant, privateKey, "cdn-key-1")
	if _, err := VerifyCDNPlaybackEvidence(context.Background(), second, provider, principal, grant, attestation, ExecutionPolicy{}); err == nil {
		t.Fatal("grant from another connector instance was accepted")
	}
}

func TestTypedCDNPlaybackEvidenceUsesConfiguredProviderAndExactPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	grant := validCDNPlaybackGrant(now)
	attestation := signedCDNPlaybackAttestation(t, grant, privateKey, "cdn-key-1")
	provider := &verifyingCDNPlaybackProvider{
		fakeCDNPlaybackProvider: fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
			kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
		}},
		keyID: "cdn-key-1", publicKey: publicKey, now: now.Add(3 * time.Minute),
	}
	instance := activeCDNPlaybackInstance(t)
	principal := scopedPrincipal(t, instance.ExecuteScope)
	policy := ExecutionPolicy{Now: func() time.Time { return now.Add(3 * time.Minute) }}

	verified, err := VerifyCDNPlaybackEvidence(context.Background(), instance, provider, principal, grant, attestation, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Valid() || verified.Binding() != grant.Binding || verified.SessionID() != grant.SessionID {
		t.Fatalf("verified evidence lost its canonical binding: %#v", verified)
	}

	tampered := attestation
	tampered.WatchedRanges = append([]CDNPlaybackRange(nil), attestation.WatchedRanges...)
	tampered.WatchedRanges[0].EndMillis--
	if _, err := VerifyCDNPlaybackEvidence(context.Background(), instance, provider, principal, grant, tampered, policy); err == nil {
		t.Fatal("tampered attestation was accepted by typed connector executor")
	}
}
