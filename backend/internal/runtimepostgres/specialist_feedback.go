package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

var feedbackUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-(?:[0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$`)

// SubmitVerifiedFeedback never accepts a claimed specialist, publication,
// rating provenance, or completed status. The completed client-owned booking
// and specialist are resolved inside PostgreSQL. Database triggers enforce
// the same invariant even against non-HTTP mutation callers.
func (c *Checker) SubmitVerifiedFeedback(clientIdentityID, bookingID string, rating int) (specialist.FeedbackSubmission, error) {
	var empty specialist.FeedbackSubmission
	if c == nil || c.db == nil {
		return empty, errors.New("PostgreSQL unavailable")
	}
	if !feedbackUUID.MatchString(bookingID) || !feedbackUUID.MatchString(clientIdentityID) || rating < 1 || rating > 5 {
		return empty, specialist.ErrFeedbackInvalid
	}
	id, err := persistentid.New()
	if err != nil {
		return empty, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	var savedID string
	err = c.db.QueryRowContext(ctx, `
		INSERT INTO specialist_verified_feedback (
			id, booking_id, specialist_id, client_identity_id, rating, state
		)
		SELECT $1::uuid, booking.id, offer.specialist_id,
			booking.client_identity_id, $2, 'PENDING'
		FROM bookings booking
		JOIN booking_slots slot ON slot.id=booking.slot_id
			AND slot.tenant_scope='specialist/live'
		JOIN specialist_bookable_offers offer ON offer.id=slot.specialist_offer_id
			AND offer.specialist_identity_id=slot.specialist_identity_id
		WHERE booking.id=$3::uuid AND booking.client_identity_id=$4::uuid
			AND booking.state='COMPLETED' AND slot.ends_at <= clock_timestamp()
		RETURNING id::text`, id, rating, bookingID, clientIdentityID).Scan(&savedID)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, specialist.ErrFeedbackNotEligible
	}
	if err != nil {
		if strings.Contains(err.Error(), "specialist_verified_feedback_booking_id_key") {
			return empty, specialist.ErrFeedbackDuplicate
		}
		return empty, fmt.Errorf("persist verified specialist feedback: %w", err)
	}
	return specialist.FeedbackSubmission{ID: savedID, State: "PENDING",
		Notice: "Оценка получена из завершённой записи. Она будет учитываться только после независимой проверки."}, nil
}

// ReviewVerifiedFeedback is a trusted operator-only moderation action, not
// an owner API. It is idempotent only for the same terminal decision.
func (c *Checker) ReviewVerifiedFeedback(feedbackID string, approved bool, reviewerRef string) error {
	if c == nil || c.db == nil {
		return errors.New("PostgreSQL unavailable")
	}
	if !feedbackUUID.MatchString(feedbackID) || strings.TrimSpace(reviewerRef) == "" {
		return specialist.ErrFeedbackInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldState string
	err = tx.QueryRowContext(ctx, `SELECT state FROM specialist_verified_feedback
		WHERE id=$1::uuid FOR UPDATE`, feedbackID).Scan(&oldState)
	if errors.Is(err, sql.ErrNoRows) {
		return specialist.ErrFeedbackNotFound
	}
	if err != nil {
		return err
	}
	newState := "REJECTED"
	if approved {
		newState = "APPROVED"
	} else if oldState == "APPROVED" || oldState == "WITHDRAWN" {
		// An operator rejection of an already visible rating is an audited
		// withdrawal; it never rewrites the source booking or historical score.
		newState = "WITHDRAWN"
	}
	if oldState == newState {
		return tx.Commit()
	}
	if oldState != "PENDING" && !(oldState == "APPROVED" && newState == "WITHDRAWN") {
		return specialist.ErrFeedbackNotEligible
	}
	_, err = tx.ExecContext(ctx, `UPDATE specialist_verified_feedback
		SET state=$2, reviewed_at=now(), reviewer_ref=$3 WHERE id=$1::uuid`,
		feedbackID, newState, reviewerRef)
	if err != nil {
		return fmt.Errorf("moderate verified feedback: %w", err)
	}
	auditID, err := persistentid.New()
	if err != nil {
		return err
	}
	oldJSON, _ := json.Marshal(map[string]string{"state": oldState})
	newJSON, _ := json.Marshal(map[string]string{"state": newState})
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_records
		(id,actor_id,action,scope,resource_ref,old_state,new_state,reason,policy_version,correlation_id)
		VALUES ($1::uuid,$2,'SPECIALIST_FEEDBACK_MODERATE','SPECIALIST',$3,
			$4::jsonb,$5::jsonb,'VERIFIED_BOOKING_FEEDBACK','reputation-v1',$6)`,
		auditID, reviewerRef, "specialist-feedback:"+feedbackID, string(oldJSON),
		string(newJSON), "specialist-feedback:"+auditID)
	if err != nil {
		return fmt.Errorf("audit verified feedback moderation: %w", err)
	}
	return tx.Commit()
}
