package runtimepostgres

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademySetLessonSchedule modifies the canonical draft version only.
// Published curriculum rules cannot be retroactively changed.
func (c *Checker) AcademySetLessonSchedule(actor, lessonID string, releaseAt, deadlineAt *time.Time) error {
	if !academyScheduleValid(releaseAt, deadlineAt) {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT v.state FROM academy_lessons l
 JOIN academy_modules m ON m.id=l.module_id
 JOIN academy_course_versions v ON v.id=m.version_id
 JOIN academy_courses c ON c.id=v.course_id
 WHERE l.id=$1::uuid AND c.author_id=$2::uuid FOR UPDATE OF v`, lessonID, actor).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "DRAFT" {
		return academy.ErrState
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_lesson_schedules(lesson_id,release_at,deadline_at)
 VALUES($1::uuid,$2,$3) ON CONFLICT (lesson_id) DO UPDATE
 SET release_at=EXCLUDED.release_at,deadline_at=EXCLUDED.deadline_at`, lessonID, releaseAt, deadlineAt)
	if err != nil {
		return err
	}
	if err = academyEvent(ctx, tx, actor, "academy/lesson/"+lessonID,
		"academy.lesson.schedule.updated", map[string]any{"lesson_id": lessonID}); err != nil {
		return err
	}
	return tx.Commit()
}

func academyScheduleValid(releaseAt, deadlineAt *time.Time) bool {
	if releaseAt == nil && deadlineAt == nil {
		return false
	}
	if releaseAt != nil && deadlineAt != nil && !deadlineAt.After(*releaseAt) {
		return false
	}
	return true
}

// AcademySetAssessmentLimits is owned by the same author and draft version.
func (c *Checker) AcademySetAssessmentLimits(actor, lessonID string, maxAttempts int, deadlineAt *time.Time) error {
	if maxAttempts < 1 || maxAttempts > 100 {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT v.state FROM academy_assessments a
 JOIN academy_lessons l ON l.id=a.lesson_id
 JOIN academy_modules m ON m.id=l.module_id
 JOIN academy_course_versions v ON v.id=m.version_id
 JOIN academy_courses c ON c.id=v.course_id
 WHERE a.lesson_id=$1::uuid AND c.author_id=$2::uuid FOR UPDATE OF v`, lessonID, actor).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "DRAFT" {
		return academy.ErrState
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_assessment_limits(lesson_id,max_attempts,submission_deadline)
 VALUES($1::uuid,$2,$3) ON CONFLICT (lesson_id) DO UPDATE
 SET max_attempts=EXCLUDED.max_attempts,submission_deadline=EXCLUDED.submission_deadline`,
		lessonID, maxAttempts, deadlineAt)
	if err != nil {
		return err
	}
	if err = academyEvent(ctx, tx, actor, "academy/lesson/"+lessonID,
		"academy.assessment.limits.updated", map[string]any{"lesson_id": lessonID, "max_attempts": maxAttempts}); err != nil {
		return err
	}
	return tx.Commit()
}
