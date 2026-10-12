package liveevents

import (
	"context"
	"time"
)

// A provider-owned media reference; APGIC does not implement SFU.
type MediaRoom struct {
	EventID            string
	OrganizationID     string
	ProviderInstanceID string
	ConferenceID       string
	IntegrationID      string
	ProviderReadyAt    time.Time
	StartsAt           time.Time
	EndsAt             time.Time
	Capacity           int
	Format             Format
	State              State
}

type MediaGrant struct {
	JoinURL   string
	ExpiresAt time.Time
}

type MediaParticipantFact struct {
	Joined           bool
	JoinCount        int
	ConnectedSeconds int64
	FirstJoinedAt    time.Time
}

// The external provider owns real conferences, WebRTC and admission tokens.
type RealMediaProvider interface {
	Prepare(context.Context, Event) (MediaRoom, error)
	Change(context.Context, MediaRoom, State) error
	Join(context.Context, MediaRoom, string, string) (MediaGrant, error)
	Moderate(context.Context, MediaRoom, string, string) error
	Attendance(context.Context, MediaRoom, string) (MediaParticipantFact, error)
}
