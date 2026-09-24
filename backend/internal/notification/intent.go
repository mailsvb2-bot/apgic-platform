package notification

import (
	"errors"
	"strings"
	"time"
)

type Channel string

const (
	ChannelPush  Channel = "PUSH"
	ChannelEmail Channel = "EMAIL"
	ChannelSMS   Channel = "SMS"
)

var ErrInvalidIntent = errors.New("invalid notification intent")

type Intent struct {
	ID             string
	BookingID      string
	Purpose        string
	IdempotencyKey string
	Transactional  bool
	DataClass      string
	CreatedAt      time.Time
}

func NewTransactional(intent Intent) (Intent, error) {
	if strings.TrimSpace(intent.ID) == "" ||
		strings.TrimSpace(intent.BookingID) == "" ||
		strings.TrimSpace(intent.Purpose) == "" ||
		strings.TrimSpace(intent.IdempotencyKey) == "" ||
		strings.TrimSpace(intent.DataClass) == "" ||
		intent.CreatedAt.IsZero() {
		return Intent{}, ErrInvalidIntent
	}
	intent.Transactional = true
	return intent, nil
}

func (i Intent) RequiresGrowthOptIn() bool {
	return !i.Transactional
}

func DeliveryIdempotencyKey(intent Intent, channel Channel, endpointRef string) (string, error) {
	if strings.TrimSpace(intent.ID) == "" ||
		(channel != ChannelPush && channel != ChannelEmail && channel != ChannelSMS) ||
		strings.TrimSpace(endpointRef) == "" {
		return "", ErrInvalidIntent
	}
	return strings.Join([]string{intent.ID, string(channel), endpointRef}, ":") , nil
}
