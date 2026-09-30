package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidServiceCredential = errors.New("invalid service credential")
	ErrServiceCredentialDenied  = errors.New("service credential is not active")
	ErrServiceSecretMismatch    = errors.New("service credential secret mismatch")
)

const minServiceSecretBytes = 32

type ServiceCredential struct {
	PrincipalID       string
	CredentialVersion string
	NotBefore         time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	secretDigest      [sha256.Size]byte
}

type AuthenticatedServicePrincipal struct {
	principal         ServicePrincipal
	credentialVersion string
	authenticatedAt   time.Time
	expiresAt         time.Time
}

func NewServiceCredential(
	principalID, credentialVersion, secret string,
	notBefore, expiresAt time.Time,
) (ServiceCredential, error) {
	principalID = strings.TrimSpace(principalID)
	credentialVersion = strings.TrimSpace(credentialVersion)
	if principalID == "" || credentialVersion == "" ||
		len([]byte(secret)) < minServiceSecretBytes ||
		notBefore.IsZero() || expiresAt.IsZero() || !expiresAt.After(notBefore) {
		return ServiceCredential{}, ErrInvalidServiceCredential
	}
	return ServiceCredential{
		PrincipalID:       principalID,
		CredentialVersion: credentialVersion,
		NotBefore:         notBefore.UTC(),
		ExpiresAt:         expiresAt.UTC(),
		secretDigest:      sha256.Sum256([]byte(secret)),
	}, nil
}

func (c ServiceCredential) Authenticate(
	principal ServicePrincipal,
	presentedSecret string,
	now time.Time,
) (AuthenticatedServicePrincipal, error) {
	now = now.UTC()
	if principal.ID != c.PrincipalID || !c.activeAt(now) {
		return AuthenticatedServicePrincipal{}, ErrServiceCredentialDenied
	}
	presentedDigest := sha256.Sum256([]byte(presentedSecret))
	if subtle.ConstantTimeCompare(c.secretDigest[:], presentedDigest[:]) != 1 {
		return AuthenticatedServicePrincipal{}, ErrServiceSecretMismatch
	}
	return AuthenticatedServicePrincipal{
		principal:         principal,
		credentialVersion: c.CredentialVersion,
		authenticatedAt:   now,
		expiresAt:         c.ExpiresAt,
	}, nil
}

func (c ServiceCredential) Rotate(
	nextVersion, nextSecret string,
	now, nextExpiresAt time.Time,
) (ServiceCredential, ServiceCredential, error) {
	now = now.UTC()
	if !c.activeAt(now) {
		return ServiceCredential{}, ServiceCredential{}, ErrServiceCredentialDenied
	}
	next, err := NewServiceCredential(c.PrincipalID, nextVersion, nextSecret, now, nextExpiresAt)
	if err != nil {
		return ServiceCredential{}, ServiceCredential{}, err
	}
	revoked := now
	previous := c
	previous.RevokedAt = &revoked
	return previous, next, nil
}

func (c ServiceCredential) Revoke(now time.Time) (ServiceCredential, error) {
	now = now.UTC()
	if !c.activeAt(now) {
		return ServiceCredential{}, ErrServiceCredentialDenied
	}
	revoked := now
	out := c
	out.RevokedAt = &revoked
	return out, nil
}

func (c ServiceCredential) activeAt(now time.Time) bool {
	if now.Before(c.NotBefore) || !now.Before(c.ExpiresAt) {
		return false
	}
	return c.RevokedAt == nil || now.Before(c.RevokedAt.UTC())
}

func (p AuthenticatedServicePrincipal) ID() string {
	return p.principal.ID
}

func (p AuthenticatedServicePrincipal) HasScope(scope string) bool {
	return p.principal.HasScope(scope)
}

func (p AuthenticatedServicePrincipal) CredentialVersion() string {
	return p.credentialVersion
}

func (p AuthenticatedServicePrincipal) ValidAt(now time.Time) bool {
	now = now.UTC()
	return !p.authenticatedAt.IsZero() && !now.Before(p.authenticatedAt) && now.Before(p.expiresAt)
}
