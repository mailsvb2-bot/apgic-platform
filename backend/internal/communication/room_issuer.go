package communication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrRoomIssuerUnavailable = errors.New("communication room issuer unavailable")
	ErrRoomGrantInvalid      = errors.New("invalid or unsafe communication room grant")
	ErrRoomGrantConflict     = errors.New("room join idempotency conflict")
)

type JoinPreparation struct {
	Decision             JoinDecision
	Role                 ParticipantRole
	EntitlementVersionID string
}

type IssuedRoomGrant struct {
	ProviderCredentialRef string    `json:"provider_credential_ref"`
	JoinURI               string    `json:"join_uri"`
	ExpiresAt             time.Time `json:"expires_at"`
}

type RoomIssuer interface {
	IssueRoomJoin(context.Context, CredentialRequest) (IssuedRoomGrant, error)
}

// HTTPRoomIssuer connects to a replaceable trusted communication capability.
// The issuer, not APGIC, owns the media room and any bearer room credential.
// APGIC persists the opaque reference, never the join URI or room token.
type HTTPRoomIssuer struct {
	endpoint      string
	token         string
	allowedOrigin string
	client        *http.Client
}

func NewHTTPRoomIssuer(endpoint, token, allowedJoinOrigin string) (*HTTPRoomIssuer, error) {
	issueURL, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || issueURL.Scheme != "https" || issueURL.Hostname() == "" ||
		issueURL.User != nil || issueURL.Fragment != "" || issueURL.RawQuery != "" {
		return nil, ErrRoomIssuerUnavailable
	}
	joinOrigin, err := url.Parse(strings.TrimSpace(allowedJoinOrigin))
	if err != nil || joinOrigin.Scheme != "https" || joinOrigin.Hostname() == "" ||
		joinOrigin.Path != "" && joinOrigin.Path != "/" ||
		joinOrigin.RawQuery != "" || joinOrigin.Fragment != "" || joinOrigin.User != nil {
		return nil, ErrRoomIssuerUnavailable
	}
	if len(strings.TrimSpace(token)) < 24 || strings.ContainsAny(token, "\r\n") {
		return nil, ErrRoomIssuerUnavailable
	}
	return &HTTPRoomIssuer{
		endpoint: issueURL.String(), token: token, allowedOrigin: joinOrigin.Scheme + "://" + joinOrigin.Host,
		client: &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (h *HTTPRoomIssuer) IssueRoomJoin(ctx context.Context, request CredentialRequest) (IssuedRoomGrant, error) {
	if h == nil || h.client == nil || request.Scope != "ROOM_JOIN" ||
		request.ProviderInstanceID == "" || request.BookingID == "" ||
		request.IdentityID == "" || request.IdempotencyKey == "" ||
		request.ExpiresAt.IsZero() {
		return IssuedRoomGrant{}, ErrRoomIssuerUnavailable
	}
	payload, err := json.Marshal(map[string]any{
		"contract_version":     "apgic.room-join.v1",
		"provider_instance_id": request.ProviderInstanceID,
		"booking_id":           request.BookingID,
		"identity_id":          request.IdentityID,
		"role":                 request.Role,
		"scope":                "ROOM_JOIN",
		"idempotency_key":      request.IdempotencyKey,
		"expires_at":           request.ExpiresAt.UTC(),
	})
	if err != nil {
		return IssuedRoomGrant{}, err
	}
	call, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return IssuedRoomGrant{}, err
	}
	call.Header.Set("Authorization", "Bearer "+h.token)
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Accept", "application/json")
	call.Header.Set("X-APGIC-Contract", "room-join-v1")
	response, err := h.client.Do(call)
	if err != nil {
		return IssuedRoomGrant{}, fmt.Errorf("%w: %v", ErrRoomIssuerUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return IssuedRoomGrant{}, ErrRoomIssuerUnavailable
	}
	var grant IssuedRoomGrant
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&grant); err != nil {
		return IssuedRoomGrant{}, ErrRoomGrantInvalid
	}
	uri, err := url.Parse(grant.JoinURI)
	if err != nil || uri.Scheme != "https" || uri.Hostname() == "" || uri.User != nil ||
		uri.Scheme+"://"+uri.Host != h.allowedOrigin ||
		grant.ProviderCredentialRef == "" || strings.TrimSpace(grant.ProviderCredentialRef) != grant.ProviderCredentialRef ||
		grant.ExpiresAt.After(request.ExpiresAt) || !grant.ExpiresAt.After(time.Now().UTC()) {
		return IssuedRoomGrant{}, ErrRoomGrantInvalid
	}
	return grant, nil
}
