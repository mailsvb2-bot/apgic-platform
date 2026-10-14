package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

var (
	ErrConsultationNotReady        = connector.ErrConsultationNotReady
	ErrConsultationOutcomeNotReady = connector.ErrConsultationOutcomeNotReady
	ErrConsultationOutcomeConflict = connector.ErrConsultationOutcomeConflict
	ErrConsultationOutcomeInvalid  = connector.ErrConsultationOutcomeInvalid
)

func (c *Checker) ReadConsultationReady(ctx context.Context, bookingID, identityID string) (*connector.ReadySnapshot, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	var v connector.ReadySnapshot
	var client, specialist sql.NullTime
	var sessionState sql.NullString
	err := c.db.QueryRowContext(ctx, `SELECT b.id::text,
        CASE WHEN b.client_identity_id=$2::uuid THEN 'CLIENT' ELSE 'SPECIALIST' END,
        b.state,b.starts_at-interval '15 minutes',b.ends_at,
        (SELECT session.state FROM consultation_sessions session WHERE session.booking_id=b.id),
        (SELECT r.ready_at FROM consultation_participant_readiness r WHERE r.booking_id=b.id AND r.identity_id=b.client_identity_id),
        (SELECT r.ready_at FROM consultation_participant_readiness r WHERE r.booking_id=b.id AND r.identity_id=bs.specialist_identity_id)
        FROM bookings b JOIN booking_slots bs ON bs.id=b.slot_id
        WHERE b.id=$1::uuid AND (b.client_identity_id=$2::uuid OR bs.specialist_identity_id=$2::uuid)`,
		bookingID, identityID).Scan(&v.BookingID, &v.Role, &v.BookingState, &v.OpensAt, &v.ClosesAt, &sessionState, &client, &specialist)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read consultation readiness: %w", err)
	}
	if sessionState.Valid {
		v.ConsultationState = sessionState.String
	}
	if client.Valid {
		t := client.Time.UTC()
		v.ClientReadyAt = &t
	}
	if specialist.Valid {
		t := specialist.Time.UTC()
		v.SpecialistReadyAt = &t
	}
	return &v, nil
}

func (c *Checker) ConfirmConsultationReady(ctx context.Context, bookingID, identityID string, now time.Time) (*connector.ReadySnapshot, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var role, state string
	var opens, closes time.Time
	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN b.client_identity_id=$2::uuid THEN 'CLIENT' ELSE 'SPECIALIST' END,
        b.state,b.starts_at-interval '15 minutes',b.ends_at
        FROM bookings b JOIN booking_slots bs ON bs.id=b.slot_id
        WHERE b.id=$1::uuid AND (b.client_identity_id=$2::uuid OR bs.specialist_identity_id=$2::uuid)
        FOR UPDATE OF b`, bookingID, identityID).Scan(&role, &state, &opens, &closes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock consultation booking: %w", err)
	}
	if state != "CONFIRMED" || now.Before(opens) || now.After(closes) {
		return nil, ErrConsultationNotReady
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO consultation_participant_readiness(booking_id,identity_id,role,ready_at)
        VALUES($1::uuid,$2::uuid,$3,$4) ON CONFLICT (booking_id,identity_id) DO NOTHING`, bookingID, identityID, role, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("persist participant readiness: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit participant readiness: %w", err)
	}
	return c.ReadConsultationReady(ctx, bookingID, identityID)
}

func (c *Checker) ReadConsultationOutcome(ctx context.Context, bookingID, identityID string) (*connector.OutcomeReport, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	var report connector.OutcomeReport
	err := c.db.QueryRowContext(ctx, `SELECT o.booking_id::text,o.progress,o.match_satisfaction,o.reported_at
      FROM consultation_client_outcomes o JOIN bookings b ON b.id=o.booking_id
      WHERE o.booking_id=$1::uuid AND o.client_identity_id=$2::uuid
        AND b.client_identity_id=$2::uuid
      ORDER BY o.reported_at DESC,o.id DESC LIMIT 1`, bookingID, identityID).
		Scan(&report.BookingID, &report.Progress, &report.MatchSatisfaction, &report.ReportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read consultation outcome: %w", err)
	}
	return &report, nil
}

func (c *Checker) RecordConsultationOutcome(ctx context.Context, bookingID, identityID, idempotencyKey string, progress, satisfaction int, now time.Time) (*connector.OutcomeReport, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	if progress < -2 || progress > 2 || satisfaction < 1 || satisfaction > 5 || len(idempotencyKey) < 16 || len(idempotencyKey) > 128 ||
		strings.TrimSpace(idempotencyKey) != idempotencyKey || now.IsZero() {
		return nil, ErrConsultationOutcomeInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT s.state
        FROM consultation_sessions s JOIN bookings b ON b.id=s.booking_id
        WHERE b.id=$1::uuid AND b.client_identity_id=$2::uuid FOR UPDATE OF b`, bookingID, identityID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock outcome consultation: %w", err)
	}
	if state != "COMPLETED" {
		return nil, ErrConsultationOutcomeNotReady
	}
	id, err := persistentid.New()
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO consultation_client_outcomes
      (id,booking_id,client_identity_id,idempotency_key,progress,match_satisfaction,reported_at)
      VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7)
      ON CONFLICT (booking_id,idempotency_key) DO NOTHING`,
		id, bookingID, identityID, idempotencyKey, progress, satisfaction, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("persist consultation outcome: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	var report connector.OutcomeReport
	err = tx.QueryRowContext(ctx, `SELECT booking_id::text,progress,match_satisfaction,reported_at
      FROM consultation_client_outcomes
      WHERE booking_id=$1::uuid AND client_identity_id=$2::uuid AND idempotency_key=$3`,
		bookingID, identityID, idempotencyKey).
		Scan(&report.BookingID, &report.Progress, &report.MatchSatisfaction, &report.ReportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConsultationOutcomeConflict
	}
	if err != nil {
		return nil, err
	}
	if report.Progress != progress || report.MatchSatisfaction != satisfaction {
		return nil, ErrConsultationOutcomeConflict
	}
	report.Idempotent = count == 0
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit consultation outcome: %w", err)
	}
	return &report, nil
}
