package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyLearnerAttempts projects only grading outcomes. Never return the raw
// response or instructor-only review evidence to a learner list endpoint.
func (c *Checker) AcademyLearnerAttempts(actor, enrollmentID string) ([]academy.Attempt, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var owner string
	err := c.db.QueryRowContext(ctx, `SELECT identity_id::text FROM academy_enrollments
 WHERE id=$1::uuid`, enrollmentID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, academy.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner != actor {
		return nil, academy.ErrDenied
	}
	rows, err := c.db.QueryContext(ctx, `SELECT a.id::text,a.enrollment_id::text,a.lesson_id::text,
 a.attempt_no,a.result,a.score,a.rubric_version,a.submitted_at,a.evaluated_at
 FROM academy_attempts a WHERE a.enrollment_id=$1::uuid
 ORDER BY a.submitted_at DESC,a.id DESC LIMIT 200`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []academy.Attempt{}
	for rows.Next() {
		var attempt academy.Attempt
		var score sql.NullInt64
		var evaluated sql.NullTime
		if err = rows.Scan(&attempt.ID, &attempt.EnrollmentID, &attempt.LessonID,
			&attempt.AttemptNo, &attempt.Result, &score, &attempt.RubricVersion,
			&attempt.SubmittedAt, &evaluated); err != nil {
			return nil, err
		}
		if score.Valid {
			value := int(score.Int64)
			attempt.Score = &value
		}
		if evaluated.Valid {
			attempt.EvaluatedAt = &evaluated.Time
		}
		result = append(result, attempt)
	}
	return result, rows.Err()
}
