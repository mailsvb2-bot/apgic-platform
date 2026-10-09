package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const (
	clientSessionCookieName = "__Host-apgic_session"
	clientSessionVersion    = "v2"
	legacySessionVersion    = "v1"
	clientSessionTTL        = 30 * 24 * time.Hour
	minSessionKeyBytes      = 32
)

var (
	ErrClientSessionKeyInvalid = errors.New("client session signing key must be at least 32 bytes")
	ErrClientSessionMissing    = errors.New("client session is missing")
	ErrClientSessionInvalid    = errors.New("client session is invalid")
	ErrClientSessionExpired    = errors.New("client session is expired")
)

type clientSessionManager struct {
	key []byte
	now func() time.Time
}

func ValidateClientSessionKey(key []byte) error {
	if len(key) < minSessionKeyBytes {
		return ErrClientSessionKeyInvalid
	}
	return nil
}

func newClientSessionManager(key []byte, now func() time.Time) (*clientSessionManager, error) {
	if err := ValidateClientSessionKey(key); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	copied := append([]byte(nil), key...)
	return &clientSessionManager{key: copied, now: now}, nil
}

func (m *clientSessionManager) issue(identityID string) (*http.Cookie, error) {
	if strings.TrimSpace(identityID) == "" {
		return nil, ErrClientSessionInvalid
	}
	sessionID, err := persistentid.New()
	if err != nil {
		return nil, fmt.Errorf("create client session id: %w", err)
	}
	expiresAt := m.now().UTC().Add(clientSessionTTL)
	payload := identityID + "|" + sessionID + "|" + strconv.FormatInt(expiresAt.Unix(), 10)
	payloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := m.sign(clientSessionVersion + "." + payloadEncoded)
	value := clientSessionVersion + "." + payloadEncoded + "." + base64.RawURLEncoding.EncodeToString(signature)

	return &http.Cookie{
		Name:     clientSessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(clientSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

func (m *clientSessionManager) identityFromRequest(r *http.Request) (string, error) {
	identityID, _, err := m.identityAndReferenceFromRequest(r)
	return identityID, err
}

func (m *clientSessionManager) identityAndReferenceFromRequest(r *http.Request) (string, string, error) {
	cookie, err := r.Cookie(clientSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return "", "", ErrClientSessionMissing
	}
	if err != nil {
		return "", "", ErrClientSessionInvalid
	}
	identityID, err := m.parse(cookie.Value)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	return identityID, "session:" + base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func (m *clientSessionManager) identityForCreate(r *http.Request) (string, *http.Cookie, error) {
	identityID, err := m.identityFromRequest(r)
	if err == nil {
		return identityID, nil, nil
	}
	if !errors.Is(err, ErrClientSessionMissing) {
		return "", nil, err
	}

	identityID, err = persistentid.New()
	if err != nil {
		return "", nil, fmt.Errorf("create client identity: %w", err)
	}
	cookie, err := m.issue(identityID)
	if err != nil {
		return "", nil, err
	}
	return identityID, cookie, nil
}

func (m *clientSessionManager) parse(value string) (string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || (parts[0] != clientSessionVersion && parts[0] != legacySessionVersion) {
		return "", ErrClientSessionInvalid
	}
	signed := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, m.sign(signed)) {
		return "", ErrClientSessionInvalid
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrClientSessionInvalid
	}
	payloadParts := strings.Split(string(rawPayload), "|")
	if strings.TrimSpace(payloadParts[0]) == "" {
		return "", ErrClientSessionInvalid
	}
	var expiresField string
	switch parts[0] {
	case clientSessionVersion:
		if len(payloadParts) != 3 || strings.TrimSpace(payloadParts[1]) == "" {
			return "", ErrClientSessionInvalid
		}
		expiresField = payloadParts[2]
	case legacySessionVersion:
		if len(payloadParts) != 2 {
			return "", ErrClientSessionInvalid
		}
		expiresField = payloadParts[1]
	default:
		return "", ErrClientSessionInvalid
	}
	expiresUnix, err := strconv.ParseInt(expiresField, 10, 64)
	if err != nil {
		return "", ErrClientSessionInvalid
	}
	if !time.Unix(expiresUnix, 0).After(m.now().UTC()) {
		return "", ErrClientSessionExpired
	}
	return payloadParts[0], nil
}

func (m *clientSessionManager) sign(value string) []byte {
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
