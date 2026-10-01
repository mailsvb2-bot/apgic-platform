package mobile

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidInstallation      = errors.New("invalid client installation")
	ErrInstallationNotFound     = errors.New("client installation not found")
	ErrPushEndpointAlreadyInUse = errors.New("push endpoint already in use")
)

type InstallationState string

const (
	InstallationActive  InstallationState = "ACTIVE"
	InstallationRevoked InstallationState = "REVOKED"
)

type ClientInstallation struct {
	ID             string            `json:"id"`
	IdentityID     string            `json:"identity_id"`
	Platform       string            `json:"platform"`
	PushEndpoint   string            `json:"push_endpoint,omitempty"`
	PushGeneration uint64            `json:"push_generation"`
	State          InstallationState `json:"state"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type InstallationStore interface {
	RegisterInstallation(ClientInstallation) (ClientInstallation, bool, error)
	RotateInstallationPushEndpoint(identityID, installationID, endpoint string, now time.Time) (ClientInstallation, bool, error)
	RevokeInstallation(identityID, installationID string, now time.Time) (ClientInstallation, bool, error)
	ListInstallations(identityID string) ([]ClientInstallation, error)
}

func NewInstallation(id, identityID, platform, pushEndpoint string, now time.Time) (ClientInstallation, error) {
	id = strings.TrimSpace(id)
	identityID = strings.TrimSpace(identityID)
	platform = strings.ToUpper(strings.TrimSpace(platform))
	pushEndpoint = strings.TrimSpace(pushEndpoint)
	if id == "" ||
		identityID == "" ||
		(platform != "IOS" && platform != "ANDROID") ||
		pushEndpoint == "" ||
		now.IsZero() {
		return ClientInstallation{}, ErrInvalidInstallation
	}
	return ClientInstallation{
		ID:             id,
		IdentityID:     identityID,
		Platform:       platform,
		PushEndpoint:   pushEndpoint,
		PushGeneration: 1,
		State:          InstallationActive,
		UpdatedAt:      now,
	}, nil
}

func (i *ClientInstallation) RotatePushEndpoint(endpoint string, now time.Time) error {
	endpoint = strings.TrimSpace(endpoint)
	if i.State != InstallationActive ||
		endpoint == "" ||
		now.IsZero() {
		return ErrInvalidInstallation
	}
	if i.PushEndpoint == endpoint {
		return nil
	}
	i.PushEndpoint = endpoint
	i.PushGeneration++
	i.UpdatedAt = now
	return nil
}

func (i *ClientInstallation) Revoke(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInstallation
	}
	if i.State == InstallationRevoked {
		return nil
	}
	i.State = InstallationRevoked
	i.PushEndpoint = ""
	i.UpdatedAt = now
	return nil
}

func (i ClientInstallation) CanReceivePush(endpoint string, generation uint64) bool {
	return i.State == InstallationActive &&
		i.PushEndpoint == endpoint &&
		i.PushGeneration == generation
}
