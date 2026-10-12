package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/growth"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const growthWriteTimeout = 3 * time.Second

// IssueSpecialistLink never believes a caller-supplied specialist/tenant or
// publication state. Both ownership and current professional eligibility are
// checked in PostgreSQL against the same canonical gate as booking.
func (c *Checker) IssueSpecialistLink(owner, offerID string, expires time.Time) (growth.Link, error) {
	if c == nil || c.db == nil {
		return growth.Link{}, errors.New("growth storage unavailable")
	}
	id, err := persistentid.New()
	if err != nil {
		return growth.Link{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	result := growth.Link{ID: id, OfferID: offerID, Source: growth.SourceSpecialist, ExpiresAt: expires.UTC(), PolicyVersion: growth.LinkPolicyVersion, State: "ACTIVE"}
	err = c.db.QueryRowContext(ctx,
		"INSERT INTO growth_links(id,issuer_identity_id,specialist_offer_id,acquisition_source,policy_version,created_at,expires_at) "+
			"SELECT $1::uuid,$2::uuid,offer.id,$3,$4,clock_timestamp(),$5 "+
			"FROM specialist_bookable_offers offer WHERE offer.id=$6::uuid "+
			"AND offer.specialist_identity_id=$2::uuid AND apgic_live_offer_valid(offer.id) "+
			"AND $5 > clock_timestamp() AND $5 <= clock_timestamp() + interval '30 days' "+
			"RETURNING (SELECT topic_id FROM specialist_bookable_offers WHERE id=$6::uuid)",
		id, owner, result.Source, result.PolicyVersion, expires.UTC(), offerID).Scan(&result.TopicID)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.Link{}, growth.ErrNotOwner
	}
	if err != nil {
		return growth.Link{}, fmt.Errorf("issue canonical specialist distribution link: %w", err)
	}
	return result, nil
}

func (c *Checker) CaptureTouch(subject, linkID string, at time.Time) (growth.Touch, error) {
	if c == nil || c.db == nil {
		return growth.Touch{}, errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return growth.Touch{}, err
	}
	defer tx.Rollback()
	// Resolve status, issuer and qualified offer inside the same transaction.
	var issuer, offer, topic, name, source string
	var expires time.Time
	err = tx.QueryRowContext(ctx,
		"SELECT l.issuer_identity_id::text,l.specialist_offer_id::text,o.topic_id,p.display_name,l.acquisition_source,l.expires_at "+
			"FROM growth_links l JOIN specialist_bookable_offers o ON o.id=l.specialist_offer_id "+
			"JOIN specialist_profiles p ON p.id=o.specialist_id "+
			"WHERE l.id=$1::uuid AND l.state='ACTIVE' AND l.expires_at>$2 AND apgic_live_offer_valid(o.id)",
		linkID, at.UTC()).Scan(&issuer, &offer, &topic, &name, &source, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.Touch{}, growth.ErrLinkUnavailable
	}
	if err != nil {
		return growth.Touch{}, fmt.Errorf("resolve growth link: %w", err)
	}
	if issuer == subject {
		return growth.Touch{}, growth.ErrSelfReferral
	}
	// Ensure a first-party session identity exists without trusting IDs in a URL.
	if _, err = tx.ExecContext(ctx, "INSERT INTO identities(id) VALUES ($1::uuid) ON CONFLICT DO NOTHING", subject); err != nil {
		return growth.Touch{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO identity_roles(identity_id,role_code) VALUES ($1::uuid,'CLIENT') ON CONFLICT DO NOTHING", subject); err != nil {
		return growth.Touch{}, err
	}
	id, err := persistentid.New()
	if err != nil {
		return growth.Touch{}, err
	}
	var touchID string
	err = tx.QueryRowContext(ctx,
		"INSERT INTO growth_touches(id,link_id,client_identity_id,recorded_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4) "+
			"ON CONFLICT (link_id,client_identity_id) DO NOTHING RETURNING id::text",
		id, linkID, subject, at.UTC()).Scan(&touchID)
	idempotent := false
	if errors.Is(err, sql.ErrNoRows) {
		idempotent = true
		err = tx.QueryRowContext(ctx, "SELECT id::text,recorded_at FROM growth_touches WHERE link_id=$1::uuid AND client_identity_id=$2::uuid", linkID, subject).Scan(&touchID, &at)
	}
	if err != nil {
		return growth.Touch{}, fmt.Errorf("persist first-party growth touch: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return growth.Touch{}, err
	}
	return growth.Touch{ID: touchID, LinkID: linkID, Source: source, OfferID: offer, TopicID: topic, SpecialistName: name, RecordedAt: at.UTC(), Idempotent: idempotent}, nil
}

func (c *Checker) RevokeSpecialistLink(owner, linkID string) error {
	if c == nil || c.db == nil {
		return errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	var found string
	err := c.db.QueryRowContext(ctx,
		"UPDATE growth_links SET state='REVOKED',revoked_at=clock_timestamp() "+
			"WHERE id=$1::uuid AND issuer_identity_id=$2::uuid AND state='ACTIVE' RETURNING id::text",
		linkID, owner).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.ErrNotOwner
	}
	return err
}

func (c *Checker) SpecialistGrowthReport(owner string) ([]growth.Report, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx,
		"SELECT l.id::text,l.specialist_offer_id::text,l.acquisition_source,l.state, "+
			"COUNT(DISTINCT t.id), COUNT(DISTINCT a.intent_id), "+
			"COUNT(DISTINCT a.intent_id) FILTER (WHERE i.state='CONFIRMED') "+
			"FROM growth_links l LEFT JOIN growth_touches t ON t.link_id=l.id "+
			"LEFT JOIN growth_intent_attribution a ON a.touch_id=t.id "+
			"LEFT JOIN help_intents i ON i.id=a.intent_id "+
			"WHERE l.issuer_identity_id=$1::uuid "+
			"GROUP BY l.id,l.specialist_offer_id,l.acquisition_source,l.state ORDER BY l.id", owner)
	if err != nil {
		return nil, fmt.Errorf("read growth funnel: %w", err)
	}
	defer rows.Close()
	out := make([]growth.Report, 0)
	for rows.Next() {
		var v growth.Report
		if err := rows.Scan(&v.LinkID, &v.OfferID, &v.Source, &v.State, &v.Touches, &v.HelpIntents, &v.ConfirmedIntents); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
