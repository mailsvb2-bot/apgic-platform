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
// IssueSpecialistLink makes one stable link per live offer. Every retry,
// browser refresh, API restart and concurrent request returns the same
// provenance until expiry or explicit revocation. SQL enforces the
// uniqueness invariant even if another instance skips the advisory lock.
func (c *Checker) IssueSpecialistLink(owner, offerID string, expires time.Time) (growth.Link, error) {
	if c == nil || c.db == nil {
		return growth.Link{}, errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return growth.Link{}, fmt.Errorf("begin growth link issuance: %w", err)
	}
	defer tx.Rollback()

	// Serialize link lifecycle for this exact owner and offer across all API
	// instances, not just within an HTTP process.
	if _, err = tx.ExecContext(ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended('apgic:growth-link:' || $1 || ':' || $2, 0))",
		owner, offerID); err != nil {
		return growth.Link{}, fmt.Errorf("lock growth link lifecycle: %w", err)
	}
	var topic string
	err = tx.QueryRowContext(ctx,
		"SELECT topic_id FROM specialist_bookable_offers WHERE id=$1::uuid "+
			"AND specialist_identity_id=$2::uuid AND apgic_live_offer_valid(id)",
		offerID, owner).Scan(&topic)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.Link{}, growth.ErrNotOwner
	}
	if err != nil {
		return growth.Link{}, fmt.Errorf("verify owned qualified offer: %w", err)
	}

	// Expired links can never become active again, but their attribution and
	// historical revenue evidence remain intact.
	_, err = tx.ExecContext(ctx,
		"UPDATE growth_links SET state='REVOKED',revoked_at=clock_timestamp() "+
			"WHERE issuer_identity_id=$1::uuid AND specialist_offer_id=$2::uuid "+
			"AND state='ACTIVE' AND expires_at<=clock_timestamp()", owner, offerID)
	if err != nil {
		return growth.Link{}, fmt.Errorf("retire expired growth links: %w", err)
	}
	link := growth.Link{
		OfferID: offerID, TopicID: topic, Source: growth.SourceSpecialist,
		PolicyVersion: growth.LinkPolicyVersion, State: "ACTIVE",
	}
	err = tx.QueryRowContext(ctx,
		"SELECT id::text,expires_at FROM growth_links "+
			"WHERE issuer_identity_id=$1::uuid AND specialist_offer_id=$2::uuid "+
			"AND state='ACTIVE' ORDER BY created_at DESC LIMIT 1",
		owner, offerID).Scan(&link.ID, &link.ExpiresAt)
	switch {
	case err == nil:
		// Existing links retain their original expiry and signature.
	case errors.Is(err, sql.ErrNoRows):
		link.ID, err = persistentid.New()
		if err != nil {
			return growth.Link{}, err
		}
		link.ExpiresAt = expires.UTC()
		err = tx.QueryRowContext(ctx,
			"INSERT INTO growth_links(id,issuer_identity_id,specialist_offer_id,acquisition_source,policy_version,expires_at) "+
				"VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6) RETURNING expires_at",
			link.ID, owner, offerID, link.Source, link.PolicyVersion, link.ExpiresAt).Scan(&link.ExpiresAt)
		if err != nil {
			return growth.Link{}, fmt.Errorf("persist unique growth link: %w", err)
		}
	default:
		return growth.Link{}, fmt.Errorf("read existing growth link: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return growth.Link{}, fmt.Errorf("commit growth link lifecycle: %w", err)
	}
	return link, nil
}

func (c *Checker) ListSpecialistLinks(owner string) ([]growth.Link, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx,
		`SELECT l.id::text,l.specialist_offer_id::text,o.topic_id,
		        l.acquisition_source,l.policy_version,l.expires_at,
		        CASE WHEN l.state='REVOKED' THEN 'REVOKED'
		             WHEN l.expires_at<=clock_timestamp() THEN 'EXPIRED'
		             WHEN NOT apgic_live_offer_valid(o.id) THEN 'UNAVAILABLE'
		             ELSE 'ACTIVE' END AS effective_state
		   FROM growth_links l
		   JOIN specialist_bookable_offers o ON o.id=l.specialist_offer_id
		  WHERE l.issuer_identity_id=$1::uuid
		  ORDER BY l.created_at DESC,l.id DESC LIMIT 100`, owner)
	if err != nil {
		return nil, fmt.Errorf("list specialist-owned distribution: %w", err)
	}
	defer rows.Close()
	out := make([]growth.Link, 0)
	for rows.Next() {
		var link growth.Link
		if err = rows.Scan(
			&link.ID, &link.OfferID, &link.TopicID, &link.Source,
			&link.PolicyVersion, &link.ExpiresAt, &link.State,
		); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
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

// Revocation is idempotent for the owner but never restores an old link.
// Revocation and issuance share one advisory lock for each owner/offer.
// Concurrent revoke/reissue cannot accidentally return a newly revoked link.
func (c *Checker) RevokeSpecialistLink(owner, linkID string) error {
	if c == nil || c.db == nil {
		return errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin growth link revocation: %w", err)
	}
	defer tx.Rollback()
	var offerID string
	err = tx.QueryRowContext(ctx,
		"SELECT specialist_offer_id::text FROM growth_links WHERE id=$1::uuid AND issuer_identity_id=$2::uuid",
		linkID, owner).Scan(&offerID)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.ErrNotOwner
	}
	if err != nil {
		return fmt.Errorf("verify growth link owner: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended('apgic:growth-link:' || $1 || ':' || $2, 0))",
		owner, offerID)
	if err != nil {
		return fmt.Errorf("lock growth link revocation: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE growth_links SET state='REVOKED',revoked_at=clock_timestamp() "+
			"WHERE id=$1::uuid AND issuer_identity_id=$2::uuid AND state='ACTIVE'",
		linkID, owner)
	if err != nil {
		return fmt.Errorf("persist growth link revocation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit growth link revocation: %w", err)
	}
	return nil
}

func (c *Checker) SpecialistGrowthReport(owner string) ([]growth.Report, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("growth storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), growthWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx,
		`SELECT l.id::text,l.specialist_offer_id::text,l.acquisition_source,l.state,
			COUNT(DISTINCT t.id),COUNT(DISTINCT a.intent_id),
			COUNT(DISTINCT a.intent_id) FILTER (WHERE i.state='CONFIRMED'),
			COUNT(DISTINCT h.hold_id),
			COUNT(DISTINCT b.id) FILTER (WHERE b.state IN ('CONFIRMED','COMPLETED')),
			COUNT(DISTINCT o.id) FILTER (WHERE e.id IS NOT NULL)
		   FROM growth_links l
		   LEFT JOIN growth_touches t ON t.link_id=l.id
		   LEFT JOIN growth_intent_attribution a ON a.touch_id=t.id
		   LEFT JOIN help_intents i ON i.id=a.intent_id
		   LEFT JOIN growth_hold_attribution h ON h.intent_id=a.intent_id
		     AND h.link_id=l.id AND h.touch_id=t.id
		   LEFT JOIN bookings b ON b.hold_id=h.hold_id
		   LEFT JOIN orders o ON o.booking_id=b.id
		   LEFT JOIN ledger_entries e ON e.economic_event_ref=o.id::text
		     AND e.credit_account_ref=o.payment_recipient_ref
		     AND e.amount_minor=o.amount_minor AND e.currency=o.currency
		     AND e.debit_account_ref LIKE 'external-provider/%/settlement'
		  WHERE l.issuer_identity_id=$1::uuid
		  GROUP BY l.id,l.specialist_offer_id,l.acquisition_source,l.state
		  ORDER BY l.id`, owner)
	if err != nil {
		return nil, fmt.Errorf("read growth funnel: %w", err)
	}
	defer rows.Close()
	out := make([]growth.Report, 0)
	for rows.Next() {
		var v growth.Report
		if err := rows.Scan(&v.LinkID, &v.OfferID, &v.Source, &v.State, &v.Touches, &v.HelpIntents, &v.ConfirmedIntents, &v.AttributedHolds, &v.ConfirmedBookings, &v.ProviderPaidOrders); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
