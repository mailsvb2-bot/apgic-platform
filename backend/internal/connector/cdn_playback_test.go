package connector

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

const (
	cdnTestSubject     = "11111111-1111-4111-8111-111111111111"
	cdnTestEnrollment  = "22222222-2222-4222-8222-222222222222"
	cdnTestEntitlement = "33333333-3333-4333-8333-333333333333"
	cdnTestCourse      = "44444444-4444-4444-8444-444444444444"
	cdnTestVersion     = "55555555-5555-4555-8555-555555555555"
	cdnTestLesson      = "66666666-6666-4666-8666-666666666666"
	cdnTestMedia       = "77777777-7777-4777-8777-777777777777"
	cdnTestSession     = "88888888-8888-4888-8888-888888888888"
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
		ContractVersion: CDNPlaybackContractVersion,
		ProviderKind:    "provider-a",
		SessionID:       cdnTestSession,
		Binding:         validCDNPlaybackBinding(),
		IssuedAt:        now.UTC(),
		ExpiresAt:       now.UTC().Add(10 * time.Minute),
	}
}

func signedCDNPlaybackAttestation(t *testing.T, grant CDNPlaybackGrant, privateKey ed25519.PrivateKey, keyID string) CDNPlaybackAttestation {
	t.Helper()
	attestation := CDNPlaybackAttestation{
		ContractVersion: CDNPlaybackContractVersion,
		ProviderKind:    grant.ProviderKind,
		SessionID:       grant.SessionID,
		Binding:         grant.Binding,
		DurationMillis:  100000,
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
	message, err := attestation.signingBytes()
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
		verified.WatchedMilliseconds() != 90000 || !verified.CompletedAt().Equal(attestation.CompletedAt) {
		t.Fatalf("verified evidence does not preserve the signed binding: %#v", verified)
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
		"changed entitlement": func(a *CDNPlaybackAttestation) { a.Binding.EntitlementID = "99999999-9999-4999-8999-999999999999" },
		"wrong key id":        func(a *CDNPlaybackAttestation) { a.KeyID = "untrusted-key" },
		"changed ranges":      func(a *CDNPlaybackAttestation) { a.WatchedRanges[0].EndMillis-- },
		"wrong session":      func(a *CDNPlaybackAttestation) { a.SessionID = "99999999-9999-4999-8999-999999999999" },
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
}

func (p *fakeCDNPlaybackProvider) IssuePlaybackGrant(_ context.Context, request CDNPlaybackRequest) (CDNPlaybackGrant, error) {
	if err := request.Validate(); err != nil {
		return CDNPlaybackGrant{}, err
	}
	return validCDNPlaybackGrant(time.Now().UTC()), nil
}

func (p *fakeCDNPlaybackProvider) VerifyPlaybackAttestation(_ context.Context, _ CDNPlaybackGrant, _ CDNPlaybackAttestation) (VerifiedCDNPlaybackAttestation, error) {
	return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
}

func TestCDNPlaybackCapabilityMustUseExistingCDNProviderClass(t *testing.T) {
	provider := &fakeCDNPlaybackProvider{fakeProvider: fakeProvider{
		kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN},
	}}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		t.Fatal(err)
	}
	ordinary := &fakeProvider{kind: "provider-a", capabilities: []CapabilityClass{CapabilityCDN}}
	if err := ValidateCDNPlaybackProvider(ordinary); err == nil {
		t.Fatal("generic CDN connector without versioned playback contract was accepted")
	}
	if err := ValidateCDNPlaybackProvider(nil); err == nil {
		t.Fatal("nil CDN provider was accepted")
	}
}

func TestCDNPlaybackRequestRejectsInvalidBinding(t *testing.T) {
	request := CDNPlaybackRequest{
		IdempotencyKey: "academy/playback/attempt-1",
		Binding:        validCDNPlaybackBinding(),
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Binding.ContentSHA256 = "not-a-digest"
	if err := request.Validate(); err == nil {
		t.Fatal("invalid media digest was accepted")
	}
}
