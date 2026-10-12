package runtimepostgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// PublicOffers is a current read projection. The same DB eligibility predicate
// used by the canonical bookable supply proves that a displayed offer remains
// published, reviewed, Product-backed and currently payable. A zero-slot
// offer is never advertised as bookable.
func (c *Checker) PublicOffers(ctx context.Context, specialistID string) ([]specialist.PublicBookableOffer, error) {
	if _, err := c.PublicProfile(ctx, specialistID); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `
SELECT offer.id::text, offer.topic_id, offer.price_minor, offer.currency,
       slot.starts_at, slot.ends_at
FROM specialist_bookable_offers offer
JOIN specialist_profiles profile ON profile.id=offer.specialist_id
JOIN booking_slots slot
  ON slot.specialist_offer_id=offer.id
 AND slot.specialist_identity_id=profile.identity_id
 AND slot.tenant_scope='specialist/live'
WHERE profile.id::text=$1
  AND apgic_live_offer_valid(offer.id)
  AND slot.exclusive
  AND slot.starts_at > clock_timestamp() + interval '15 minutes'
  AND NOT EXISTS (
    SELECT 1 FROM booking_holds hold
     WHERE hold.slot_id=slot.id AND hold.state='ACTIVE'
       AND hold.expires_at>clock_timestamp()
  )
  AND NOT EXISTS (
    SELECT 1 FROM bookings booking
     WHERE booking.slot_id=slot.id
       AND booking.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  )
ORDER BY offer.topic_id, offer.id, slot.starts_at
LIMIT 100`, specialistID)
	if err != nil {
		return nil, fmt.Errorf("read public bookable offers: %w", err)
	}
	defer rows.Close()
	result := make([]specialist.PublicBookableOffer, 0)
	positions := make(map[string]int)
	for rows.Next() {
		var id, topic, currency string
		var priceMinor int64
		var starts, ends time.Time
		if err := rows.Scan(&id, &topic, &priceMinor, &currency, &starts, &ends); err != nil {
			return nil, fmt.Errorf("scan public bookable offer: %w", err)
		}
		index, ok := positions[id]
		if !ok {
			index = len(result)
			positions[id] = index
			result = append(result, specialist.PublicBookableOffer{
				ID:         id,
				TopicID:    topic,
				PriceMinor: priceMinor,
				Currency:   currency,
				Slots:      make([]specialist.PublicBookableSlot, 0, 4),
			})
		}
		if len(result[index].Slots) < 4 {
			result[index].Slots = append(result[index].Slots, specialist.PublicBookableSlot{
				SlotID:   publicSlotRef(specialistID, starts),
				StartsAt: starts,
				EndsAt:   ends,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public bookable offers: %w", err)
	}
	return result, nil
}
