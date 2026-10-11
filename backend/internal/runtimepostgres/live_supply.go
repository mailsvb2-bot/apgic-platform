package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

const liveSupplyScope = "specialist/live"

// CreateBookableOffer explicitly binds a real price to a reviewed publication
// AND an existing, published Organization/Product owned by the specialist.
// Neither a profile alone nor conformance catalog can create a saleable offer.
func (c *Checker) CreateBookableOffer(identityID, productID, topicID string, priceMinor int64) (specialist.BookableOffer, error) {
	var empty specialist.BookableOffer
	if c == nil || c.db == nil {
		return empty, errors.New("PostgreSQL unavailable")
	}
	topicID, err := specialist.NormalizeTopic(topicID)
	if err != nil || priceMinor <= 0 || priceMinor > 10000000000 {
		return empty, specialist.ErrOfferInvalid
	}
	offerID, err := persistentid.New()
	if err != nil {
		return empty, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, fmt.Errorf("begin priced offer: %w", err)
	}
	defer tx.Rollback()
	var profileID string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM specialist_profiles WHERE identity_id=$1::uuid FOR UPDATE`, identityID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, specialist.ErrProfileNotFound
	}
	if err != nil {
		return empty, err
	}
	// Never create a product, organization or financial owner implicitly.
	var permitted bool
	err = tx.QueryRowContext(ctx, `
        SELECT EXISTS (
          SELECT 1 FROM products p
          JOIN organizations o ON o.id=p.owner_id AND o.status='ACTIVE'
          JOIN organization_memberships member
            ON member.organization_id=o.id AND member.identity_id=$1::uuid AND member.status='ACTIVE'
          JOIN organization_ownerships owner
            ON owner.organization_id=o.id AND owner.identity_id=$1::uuid AND owner.status='ACTIVE'
          JOIN organization_directions d
            ON d.id=p.organization_direction_id AND d.organization_id=o.id AND d.status='ACTIVE'
          WHERE p.id=$2::uuid AND p.owner_type='ORGANIZATION' AND p.status='PUBLISHED'
        )`, identityID, productID).Scan(&permitted)
	if err != nil {
		return empty, fmt.Errorf("check priced offer product ownership: %w", err)
	}
	if !permitted {
		return empty, specialist.ErrOfferNotAuthorized
	}
	_, err = tx.ExecContext(ctx, `
      INSERT INTO specialist_bookable_offers
        (id,specialist_id,specialist_identity_id,product_id,topic_id,price_minor,currency,state)
      VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,'RUB','ACTIVE')`,
		offerID, profileID, identityID, productID, topicID, priceMinor)
	if err != nil {
		if strings.Contains(err.Error(), "specialist_one_active_bookable_offer_per_topic") {
			return empty, specialist.ErrOfferInvalid
		}
		if strings.Contains(err.Error(), "live offer requires reviewed publication and authorized Product") {
			return empty, specialist.ErrOfferNotEligible
		}
		return empty, fmt.Errorf("insert priced offer: %w", err)
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT apgic_live_offer_valid($1::uuid)`, offerID).Scan(&valid); err != nil {
		return empty, fmt.Errorf("validate priced offer: %w", err)
	}
	if !valid {
		return empty, specialist.ErrOfferNotEligible
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return specialist.BookableOffer{ID: offerID, ProductID: productID, TopicID: topicID, PriceMinor: priceMinor, Currency: "RUB", State: "ACTIVE"}, nil
}

func (c *Checker) ListBookableOffers(identityID string) ([]specialist.BookableOffer, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("PostgreSQL unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `
        SELECT id::text,product_id::text,topic_id,price_minor,currency,state
        FROM specialist_bookable_offers WHERE specialist_identity_id=$1::uuid
        ORDER BY created_at,id`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	offers := make([]specialist.BookableOffer, 0)
	for rows.Next() {
		var item specialist.BookableOffer
		if err := rows.Scan(&item.ID, &item.ProductID, &item.TopicID, &item.PriceMinor, &item.Currency, &item.State); err != nil {
			return nil, err
		}
		offers = append(offers, item)
	}
	return offers, rows.Err()
}

// PauseBookableOffer is the only way to withdraw an immutable price version.
// Historical holds/bookings are preserved for owner readback and reconciliation.
func (c *Checker) PauseBookableOffer(identityID, offerID string) (specialist.BookableOffer, error) {
	var empty specialist.BookableOffer
	if c == nil || c.db == nil {
		return empty, errors.New("PostgreSQL unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var item specialist.BookableOffer
	var ownerID string
	err = tx.QueryRowContext(ctx, `SELECT id::text, specialist_identity_id::text, product_id::text,
		topic_id, price_minor, currency, state
		FROM specialist_bookable_offers WHERE id = $1::uuid FOR UPDATE`, offerID).
		Scan(&item.ID, &ownerID, &item.ProductID, &item.TopicID, &item.PriceMinor, &item.Currency, &item.State)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && ownerID != identityID) {
		return empty, specialist.ErrOfferNotAuthorized
	}
	if err != nil {
		return empty, err
	}
	if item.State == "ACTIVE" {
		_, err = tx.ExecContext(ctx, `UPDATE specialist_bookable_offers
			SET state='PAUSED', paused_at=now()
			WHERE id=$1::uuid AND specialist_identity_id=$2::uuid AND state='ACTIVE'`, offerID, identityID)
		if err != nil {
			return empty, fmt.Errorf("pause specialist priced offer: %w", err)
		}
		item.State = "PAUSED"
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return item, nil
}

func (c *Checker) AddBookableSlot(identityID, offerID string, start, end time.Time) (specialist.BookableSlot, error) {
	var empty specialist.BookableSlot
	if c == nil || c.db == nil {
		return empty, errors.New("PostgreSQL unavailable")
	}
	start, end = start.UTC(), end.UTC()
	if start.IsZero() || end.IsZero() || !end.After(start) ||
		end.Sub(start) > 4*time.Hour || !start.After(time.Now().UTC().Add(15*time.Minute)) {
		return empty, specialist.ErrSlotInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var profileID, actualIdentity, topic string
	err = tx.QueryRowContext(ctx, `
        SELECT specialist_id::text,specialist_identity_id::text,topic_id
          FROM specialist_bookable_offers WHERE id=$1::uuid FOR UPDATE`, offerID).
		Scan(&profileID, &actualIdentity, &topic)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, specialist.ErrOfferNotAuthorized
	}
	if err != nil {
		return empty, err
	}
	if actualIdentity != identityID {
		return empty, specialist.ErrOfferNotAuthorized
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT apgic_live_offer_valid($1::uuid)`, offerID).Scan(&valid); err != nil {
		return empty, err
	}
	if !valid {
		return empty, specialist.ErrOfferNotEligible
	}
	publicRef := publicSlotRef(profileID, start)
	slotID, err := catalogSlotUUID(publicRef)
	if err != nil {
		return empty, err
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO booking_slots
          (id,specialist_identity_id,tenant_scope,starts_at,ends_at,exclusive,specialist_offer_id)
        VALUES ($1::uuid,$2::uuid,'specialist/live',$3,$4,true,$5::uuid)`,
		slotID, identityID, start, end, offerID)
	if err != nil {
		if strings.Contains(err.Error(), "overlapping live specialist slot") ||
			strings.Contains(err.Error(), "booking_slots_live_identity_start_unique") ||
			strings.Contains(err.Error(), "booking_slots_pkey") {
			return empty, specialist.ErrSlotConflict
		}
		if strings.Contains(err.Error(), "live slot requires active authorized priced offer") {
			return empty, specialist.ErrOfferNotEligible
		}
		return empty, fmt.Errorf("insert live slot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return specialist.BookableSlot{ID: publicRef, OfferID: offerID, StartsAt: start, EndsAt: end, Exclusive: true}, nil
}

// BookableOffers returns only real, currently eligible offers with future
// unheld and unbooked canonical slots, never the four test specialists.
// A repeatable read ensures profile, offer, price and availability are
// taken from one self-consistent PostgreSQL snapshot.
func (c *Checker) BookableOffers() ([]demand.BookableOffer, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("PostgreSQL unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
      SELECT offer.id::text, sp.id::text,sp.identity_id::text,sp.display_name,
             sp.profession_code,offer.topic_id,offer.price_minor,offer.currency,
             to_json(cap.evidence_refs)::text,
             slot.starts_at,slot.ends_at,slot.exclusive
        FROM specialist_bookable_offers offer
        JOIN specialist_profiles sp ON sp.id=offer.specialist_id
        JOIN specialist_capabilities cap
          ON cap.specialist_id=sp.id AND cap.topic_id=offer.topic_id
        JOIN booking_slots slot
          ON slot.specialist_offer_id=offer.id AND slot.tenant_scope='specialist/live'
         AND slot.specialist_identity_id=sp.identity_id
       WHERE apgic_live_offer_valid(offer.id)
         AND slot.exclusive
         AND slot.starts_at > clock_timestamp() + interval '15 minutes'
         AND NOT EXISTS (
           SELECT 1 FROM booking_holds h
            WHERE h.slot_id=slot.id AND h.state='ACTIVE'
              AND h.expires_at > clock_timestamp()
         )
         AND NOT EXISTS (
           SELECT 1 FROM bookings b
            WHERE b.slot_id=slot.id AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
         )
       ORDER BY offer.id,slot.starts_at`)
	if err != nil {
		return nil, fmt.Errorf("read live bookable offers: %w", err)
	}
	result := make([]demand.BookableOffer, 0)
	positions := make(map[string]int)
	for rows.Next() {
		var offerID, profileID, identityID, displayName, profession, topic, currency, evidenceJSON string
		var price int64
		var starts, ends time.Time
		var exclusive bool
		if err := rows.Scan(&offerID, &profileID, &identityID, &displayName, &profession,
			&topic, &price, &currency, &evidenceJSON, &starts, &ends, &exclusive); err != nil {
			rows.Close()
			return nil, err
		}
		pos, exists := positions[offerID]
		if !exists {
			var refs []string
			if err := json.Unmarshal([]byte(evidenceJSON), &refs); err != nil {
				rows.Close()
				return nil, err
			}
			profile, err := marketplace.NewSpecialistProfile(profileID, identityID, displayName)
			if err != nil {
				rows.Close()
				return nil, err
			}
			cap, err := marketplace.NewCapability(topic, marketplace.EvidenceAPGICVerified, refs...)
			if err != nil {
				rows.Close()
				return nil, err
			}
			profile.Profession = profession
			profile.Complete = true
			profile.Review = marketplace.ReviewApproved
			profile.PublishState = marketplace.PublishPublished
			profile.PublishedTopics = []string{topic}
			profile.AddCapability(cap)
			pos = len(result)
			positions[offerID] = pos
			result = append(result, demand.BookableOffer{
				ID: offerID, Profile: *profile, Topic: topic, PriceMinor: price, Currency: currency,
			})
		}
		result[pos].Slots = append(result[pos].Slots, demand.Slot{
			ID: publicSlotRef(profileID, starts), SpecialistID: profileID, OfferID: offerID,
			Topic: topic, StartsAt: starts, EndsAt: ends, Exclusive: exclusive,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
