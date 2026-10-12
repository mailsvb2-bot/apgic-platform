package growth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSigningKey      = errors.New("growth token signing key requires at least 32 bytes")
	ErrTokenInvalid    = errors.New("invalid growth attribution token")
	ErrTokenExpired    = errors.New("expired growth attribution token")
	ErrLinkUnavailable = errors.New("growth link unavailable or qualification revoked")
	ErrNotOwner        = errors.New("growth link requires an eligible specialist offer owner")
	ErrSelfReferral    = errors.New("self referral is not an eligible acquisition")
)

const tokenVersion = "g1"
const LinkPolicyVersion = "growth-specialist-link-v1"
const SourceSpecialist = "SPECIALIST_SOURCED"
const TouchWindow = 30 * 24 * time.Hour

// Signer is deliberately a first-party opaque link reference. No client-supplied
// tenant/specialist/commission/source claim is accepted as business truth.
type Signer struct{ key []byte }

func NewSigner(key []byte) (*Signer, error) {
	if len(key) < 32 {
		return nil, ErrSigningKey
	}
	return &Signer{key: append([]byte(nil), key...)}, nil
}

func (s *Signer) Issue(linkID string, expiresAt time.Time) (string, error) {
	if s == nil || strings.TrimSpace(linkID) == "" || expiresAt.IsZero() {
		return "", ErrTokenInvalid
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(linkID + "|" + strconv.FormatInt(expiresAt.UTC().Unix(), 10)))
	signed := tokenVersion + "." + payload
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Signer) Verify(token string, now time.Time) (string, error) {
	if s == nil || now.IsZero() || len(token) > 512 {
		return "", ErrTokenInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenVersion {
		return "", ErrTokenInvalid
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac.Sum(nil), provided) {
		return "", ErrTokenInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrTokenInvalid
	}
	fields := strings.Split(string(data), "|")
	if len(fields) != 2 || fields[0] == "" {
		return "", ErrTokenInvalid
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || exp <= 0 {
		return "", ErrTokenInvalid
	}
	if !time.Unix(exp, 0).After(now.UTC()) {
		return "", ErrTokenExpired
	}
	return fields[0], nil
}
