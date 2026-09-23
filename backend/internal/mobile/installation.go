package mobile

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidInstallation = errors.New("invalid client installation")

type InstallationState string

const (
	InstallationActive  InstallationState = "ACTIVE"
	InstallationRevoked InstallationState = "REVOKED"
)

type ClientInstallation struct {
	ID             string
	IdentityID     string
	Platform       string
	PushEndpoint   string
	PushGeneration uint64
	State          InstallationState
	UpdatedAt      time.Time
}

func NewInstallation(id, identityID, platform, pushEndpoint string, now time.Time) (ClientInstallation, error) {
	if strings.TrimSpace(id) == "" ||
		strings.TrimSpace(identityID) == "" ||
		strings.TrimSpace(platform) == "" ||
		strings.TrimSpace(pushEndpoint) == "" ||
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
	if i.State != InstallationActive ||
		strings.TrimSpace(endpoint) == "" ||
		now.IsZero() {
		return ErrInvalidInstallation
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
