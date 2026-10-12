// Package instant defines the strictly prepayment Duty/Now boundary.
// Neither an accepted offer nor a reservation grants access to a live room.
package instant

import (
	"errors"
	"regexp"
	"time"
)

const (
	StateOffline          = "OFFLINE"
	StateAvailable        = "AVAILABLE_NOW"
	StateOffered          = "OFFERED"
	StateReserved         = "RESERVED"
	StateConsulting       = "IN_CONSULTATION"
	StateCooldown         = "COOLDOWN"
	DispatchUnfilled      = "UNFILLED"
	DispatchOffered       = "OFFERED"
	DispatchAccepted      = "ACCEPTED"
	DispatchDeclined      = "DECLINED"
	DispatchExpired       = "EXPIRED"
	ActionPaymentRequired = "PAY_REAL_PROVIDER_REQUIRED"
	ActionWaiting         = "WAIT_SPECIALIST_RESPONSE"
	ActionRetry           = "RETRY_OR_BOOK_LATER"
)

var (
	ErrInvalid     = errors.New("duty request invalid")
	ErrNotFound    = errors.New("duty resource not found")
	ErrConflict    = errors.New("duty state conflict")
	ErrNotEligible = errors.New("duty qualification or intent not eligible")
	ErrUnavailable = errors.New("duty store unavailable")
)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidID(value string) bool { return uuidPattern.MatchString(value) }

type Presence struct {
	SpecialistIdentityID string     `json:"specialist_identity_id"`
	OfferID              string     `json:"offer_id,omitempty"`
	State                string     `json:"state"`
	AvailableUntil       *time.Time `json:"available_until,omitempty"`
	Version              int64      `json:"version"`
}

type Dispatch struct {
	ID                   string     `json:"id"`
	RequestKey           string     `json:"request_key"`
	ClientIdentityID     string     `json:"client_identity_id"`
	HelpIntentID         string     `json:"help_intent_id"`
	SpecialistIdentityID string     `json:"specialist_identity_id,omitempty"`
	OfferID              string     `json:"offer_id,omitempty"`
	TopicID              string     `json:"topic_id,omitempty"`
	PriceMinor           int64      `json:"price_minor,omitempty"`
	Currency             string     `json:"currency,omitempty"`
	State                string     `json:"state"`
	OfferedAt            *time.Time `json:"offered_at,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	RespondedAt          *time.Time `json:"responded_at,omitempty"`
	NextAction           string     `json:"next_action"`
	RoomReady            bool       `json:"room_ready"`
}

func (d *Dispatch) Normalize() {
	d.RoomReady = false
	switch d.State {
	case DispatchOffered:
		d.NextAction = ActionWaiting
	case DispatchAccepted:
		d.NextAction = ActionPaymentRequired
	default:
		d.NextAction = ActionRetry
	}
}

type Store interface {
	SetPresence(identityID, offerID string, available bool) (Presence, error)
	GetPresence(identityID string) (Presence, error)
	Request(identityID, intentID, requestKey string) (Dispatch, error)
	GetDispatch(id, identityID string) (Dispatch, error)
	ActiveDispatch(identityID string) (Dispatch, error)
	Answer(id, specialistID string, accept bool) (Dispatch, error)
	Inbox(specialistID string) ([]Dispatch, error)
}
