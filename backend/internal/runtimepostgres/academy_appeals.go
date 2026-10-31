package runtimepostgres

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademySubmitAppeal is an authenticated learner operation. The PostgreSQL
// appeal trigger independently verifies the learner owns a graded attempt.
func (c *Checker) AcademySubmitAppeal(actor, attemptID, reason string) (academy.Appeal, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 10 || len([]rune(reason)) > 2000 {
		return academy.Appeal{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Appeal{}, err
	}
	defer tx.Rollback()
	var learner, result string
	err = tx.QueryRowContext(ctx, `SELECT e.identity_id::text,a.result
 FROM academy_attempts a JOIN academy_enrollments e ON e.id=a.enrollment_id
 WHERE a.id=$1::uuid FOR UPDATE OF a`, attemptID).Scan(&learner, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Appeal{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Appeal{}, err
	}
	if learner != actor {
		return academy.Appeal{}, academy.ErrDenied
	}
	if result != "FAILED" && result != "PASSED" {
		return academy.Appeal{}, academy.ErrState
	}
	id, err := academyID()
	if err != nil {
		return academy.Appeal{}, err
	}
	var submitted time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_assessment_appeals(id,attempt_id,submitted_by,reason)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4)
 ON CONFLICT (attempt_id) DO NOTHING RETURNING submitted_at`,
		id, attemptID, actor, reason).Scan(&submitted)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Appeal{}, academy.ErrConflict
	}
	if err != nil {
		return academy.Appeal{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/attempt/"+attemptID,
		"academy.assessment.appeal.submitted", map[string]any{"appeal_id": id}); err != nil {
		return academy.Appeal{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Appeal{}, err
	}
	return academy.Appeal{ID: id, AttemptID: attemptID, State: "SUBMITTED", Reason: reason, SubmittedAt: submitted}, nil
}

func (c *Checker) AcademyMyAppeals(actor string) ([]academy.Appeal, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT ap.id::text,ap.attempt_id::text,ap.state,ap.reason,
 ap.submitted_at,ap.resolved_at
 FROM academy_assessment_appeals ap
 JOIN academy_attempts a ON a.id=ap.attempt_id
 JOIN academy_enrollments e ON e.id=a.enrollment_id
 WHERE e.identity_id=$1::uuid ORDER BY ap.submitted_at DESC LIMIT 200`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []academy.Appeal{}
	for rows.Next() {
		var item academy.Appeal
		var resolved sql.NullTime
		if err = rows.Scan(&item.ID, &item.AttemptID, &item.State, &item.Reason,
			&item.SubmittedAt, &resolved); err != nil {
			return nil, err
		}
		if resolved.Valid {
			item.ResolvedAt = &resolved.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
