package specialist

import (
	"context"
	"time"
)

// PublicBookableSlot is a currently eligible, future, exclusive slot projection,
// not a hold or booking guarantee. All private client booking data is excluded.
type PublicBookableSlot struct {
	SlotID   string    `json:"slot_id"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// PublicBookableOffer reflects a real Organization/Product-backed immutable
// price version; inactive/unqualified/unavailable offers are not returned.
type PublicBookableOffer struct {
	ID         string               `json:"id"`
	TopicID    string               `json:"topic_id"`
	PriceMinor int64                `json:"price_minor"`
	Currency   string               `json:"currency"`
	Slots      []PublicBookableSlot `json:"slots"`
}

type PublicBookableOfferStore interface {
	PublicOffers(ctx context.Context, specialistID string) ([]PublicBookableOffer, error)
}
