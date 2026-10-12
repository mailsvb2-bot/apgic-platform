package clientpersonal

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var ErrInvalidCursor = errors.New("invalid personal history cursor")
var ErrInvalidPreferences = errors.New("invalid personal preferences")
var ErrIdentityNotFound = errors.New("client identity not found")

const MaxHistoryLimit = 50

type Preferences struct {
	DisplayName string     `json:"display_name"`
	Locale      string     `json:"locale"`
	TimeZone    string     `json:"time_zone"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func DefaultPreferences() Preferences {
	return Preferences{DisplayName: "", Locale: "ru", TimeZone: "Europe/Moscow"}
}

func ValidatePreferences(p Preferences) error {
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	if len([]rune(p.DisplayName)) > 80 || strings.ContainsAny(p.DisplayName, "\r\n\x00") ||
		(p.Locale != "ru" && p.Locale != "en") || len(p.TimeZone) > 64 {
		return ErrInvalidPreferences
	}
	if _, err := time.LoadLocation(p.TimeZone); err != nil {
		return ErrInvalidPreferences
	}
	return nil
}

type Booking struct {
	ID                   string    `json:"booking_id"`
	State                string    `json:"state"`
	StartsAt             time.Time `json:"starts_at"`
	EndsAt               time.Time `json:"ends_at"`
	SpecialistName       string    `json:"specialist_name"`
	SpecialistProfession string    `json:"specialist_profession"`
	ConsultationState    *string   `json:"consultation_state,omitempty"`
	OrderID              *string   `json:"order_id,omitempty"`
}

type Request struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Topics    []string  `json:"topics"`
	CreatedAt time.Time `json:"created_at"`
}

type View struct {
	IdentityID        string      `json:"identity_id"`
	AsOf              time.Time   `json:"as_of"`
	Preferences       Preferences `json:"preferences"`
	Upcoming          []Booking   `json:"upcoming"`
	History           []Booking   `json:"history"`
	Requests          []Request   `json:"requests"`
	NextHistoryCursor string      `json:"next_history_cursor,omitempty"`
}

type HistoryCursor struct {
	StartsAt  time.Time
	BookingID string
}

func EncodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func DecodeCursor(raw string) (*HistoryCursor, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 200 {
		return nil, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 || len(parts[1]) != 36 || !uuidLike(parts[1]) {
		return nil, ErrInvalidCursor
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, ErrInvalidCursor
	}
	return &HistoryCursor{StartsAt: at, BookingID: parts[1]}, nil
}
func uuidLike(s string) bool {
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
