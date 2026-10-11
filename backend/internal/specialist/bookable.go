package specialist

import (
	"errors"
	"time"
)

var (
	ErrOfferInvalid       = errors.New("priced offer must have a valid price, topic and published product")
	ErrOfferNotAuthorized = errors.New("specialist does not own this product or live offer")
	ErrOfferNotEligible   = errors.New("specialist or product is not eligible for publication")
	ErrSlotInvalid        = errors.New("exclusive consultation slot must be future and have valid bounds")
	ErrSlotConflict       = errors.New("a live specialist slot overlaps another live slot")
)

type BookableOffer struct {
	ID         string `json:"id"`
	ProductID  string `json:"product_id"`
	TopicID    string `json:"topic_id"`
	PriceMinor int64  `json:"price_minor"`
	Currency   string `json:"currency"`
	State      string `json:"state"`
}

type BookableSlot struct {
	ID        string    `json:"id"`
	OfferID   string    `json:"offer_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Exclusive bool      `json:"exclusive"`
}

// OfferStore is an authenticated server-side mutation owner. Client claims
// about specialist identity, product owner or verification are never trusted.
type OfferStore interface {
	CreateBookableOffer(identityID, productID, topicID string, priceMinor int64) (BookableOffer, error)
	AddBookableSlot(identityID, offerID string, start, end time.Time) (BookableSlot, error)
	ListBookableOffers(identityID string) ([]BookableOffer, error)
	PauseBookableOffer(identityID, offerID string) (BookableOffer, error)
}
