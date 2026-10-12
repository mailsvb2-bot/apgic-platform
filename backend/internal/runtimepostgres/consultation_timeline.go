package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

func (c *Checker) ReadConsultationTimeline(ctx context.Context, bookingID, identityID string) (*connector.ConsultationTimeline, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil { return nil, err }
	defer tx.Rollback()

	var sessionID string
	result := &connector.ConsultationTimeline{
		Facts: []connector.ConsultationTimelineFact{},
		Recovery: []connector.ConsultationRecoveryFact{},
	}
	err = tx.QueryRowContext(ctx, `SELECT s.id::text,b.id::text,b.state,s.state
		FROM consultation_sessions s JOIN bookings b ON b.id=s.booking_id
		JOIN booking_slots bs ON bs.id=b.slot_id
		WHERE b.id=$1::uuid
		  AND (b.client_identity_id=$2::uuid OR bs.specialist_identity_id=$2::uuid)`,
		bookingID, identityID).
		Scan(&sessionID, &result.BookingID, &result.BookingState, &result.ConsultationState)
	if errors.Is(err, sql.ErrNoRows) { return nil, nil }
	if err != nil { return nil, fmt.Errorf("read consultation timeline header: %w", err) }

	rows, err := tx.QueryContext(ctx, `SELECT fact_type,role,evidence_ref,occurred_at
		FROM consultation_lifecycle_facts
		WHERE session_id=$1::uuid ORDER BY occurred_at ASC,id ASC`, sessionID)
	if err != nil { return nil, fmt.Errorf("read consultation timeline evidence: %w", err) }
	for rows.Next() {
		var fact connector.ConsultationTimelineFact
		if err = rows.Scan(&fact.FactType, &fact.Role, &fact.EvidenceRef, &fact.OccurredAt); err != nil { break }
		result.Facts = append(result.Facts, fact)
	}
	if err == nil { err = rows.Err() }
	rows.Close()
	if err != nil { return nil, fmt.Errorf("read consultation timeline facts: %w", err) }

	decisions, err := tx.QueryContext(ctx, `SELECT action,reason_code,COALESCE(followup_path_ref,''),decided_at
		FROM consultation_recovery_decisions
		WHERE session_id=$1::uuid ORDER BY decided_at ASC,id ASC`, sessionID)
	if err != nil { return nil, fmt.Errorf("read consultation recovery history: %w", err) }
	for decisions.Next() {
		var fact connector.ConsultationRecoveryFact
		if err = decisions.Scan(&fact.Action,&fact.ReasonCode,&fact.FollowupPathRef,&fact.DecidedAt);err != nil {break}
		result.Recovery = append(result.Recovery, fact)
	}
	if err == nil {err = decisions.Err()}
	decisions.Close()
	if err != nil {return nil, fmt.Errorf("read consultation recovery facts: %w", err)}
	result.RequiresFollowup = result.ConsultationState == "TECHNICAL_FAILURE"
	if err = tx.Commit(); err != nil {return nil, fmt.Errorf("commit consultation timeline snapshot: %w", err)}
	return result, nil
}
