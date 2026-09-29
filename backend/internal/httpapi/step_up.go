package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	stepUpCookieName = "__Host-apgic_step_up"
	stepUpVersion    = "su1"
	stepUpTTL        = 10 * time.Minute
)

var ErrStepUpAssertionInvalid = errors.New("step-up assertion is invalid")

type stepUpManager struct {
	key []byte
	now func() time.Time
}

func newStepUpManager(key []byte, now func() time.Time) (*stepUpManager, error) {
	if err := ValidateClientSessionKey(key); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &stepUpManager{key: append([]byte(nil), key...), now: now}, nil
}

func (m *stepUpManager) issue(identityID string, issuedAt time.Time) (*http.Cookie, error) {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" || issuedAt.IsZero() {
		return nil, ErrStepUpAssertionInvalid
	}
	expiresAt := issuedAt.UTC().Add(stepUpTTL)
	raw := identityID + "|" + strconv.FormatInt(issuedAt.UTC().Unix(), 10) + "|" + strconv.FormatInt(expiresAt.Unix(), 10)
	payload := base64.RawURLEncoding.EncodeToString([]byte(raw))
	signed := stepUpVersion + "." + payload
	value := signed + "." + base64.RawURLEncoding.EncodeToString(m.sign(signed))
	return &http.Cookie{
		Name:     stepUpCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(stepUpTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

func (m *stepUpManager) issuedAtFromRequest(r *http.Request, identityID string) *time.Time {
	cookie, err := r.Cookie(stepUpCookieName)
	if err != nil {
		return nil
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || parts[0] != stepUpVersion {
		return nil
	}
	signed := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, m.sign(signed)) {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 3 || fields[0] != identityID {
		return nil
	}
	issuedUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil
	}
	expiresUnix, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || !time.Unix(expiresUnix, 0).After(m.now().UTC()) {
		return nil
	}
	issuedAt := time.Unix(issuedUnix, 0).UTC()
	return &issuedAt
}

func (m *stepUpManager) sign(value string) []byte {
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte("apgic-step-up|" + value))
	return mac.Sum(nil)
}
