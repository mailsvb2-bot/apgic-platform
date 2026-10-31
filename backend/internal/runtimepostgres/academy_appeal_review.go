package runtimepostgres

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyReviewAppeal performs only the prescribed state transition. The
// reviewer grant is provisioned outside HTTP; SQL independently checks it.
func (c *Checker) AcademyReviewAppeal(actor, appealID, decision, evidenceRef string) error {
	if actor == "" || appealID == "" {
		return academy.ErrInvalid
	}
	if decision != "UNDER_REVIEW" && decision != "UPHELD" && decision != "REJECTED" {
		return academy.ErrInvalid
	}
	evidenceRef = strings.TrimSpace(evidenceRef)
	if (decision == "UPHELD" || decision == "REJECTED") && evidenceRef == "" {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, learner, author string
	var reviewer sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT ap.state,e.identity_id::text,c.author_id::text,
 ap.independent_reviewer_id::text
 FROM academy_assessment_appeals ap
 JOIN academy_attempts a ON a.id=ap.attempt_id
 JOIN academy_enrollments e ON e.id=a.enrollment_id
 JOIN academy_course_versions v ON v.id=e.version_id
 JOIN academy_courses c ON c.id=v.course_id
 WHERE ap.id=$1::uuid FOR UPDATE OF ap`, appealID).Scan(&state, &learner, &author, &reviewer)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrNotFound
	}
	if err != nil {
		return err
	}
	if actor == learner || actor == author {
		return academy.ErrDenied
	}
	var granted bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM academy_appeal_reviewer_grants
 WHERE reviewer_identity_id=$1::uuid AND revoked_at IS NULL
 FOR UPDATE)`, actor).Scan(&granted)
	if err != nil {
		return err
	}
	if !granted {
		return academy.ErrDenied
	}
	if decision == "UNDER_REVIEW" && state != "SUBMITTED" ||
		decision != "UNDER_REVIEW" && state != "UNDER_REVIEW" ||
		reviewer.Valid && reviewer.String != actor {
		return academy.ErrState
	}
	var resolution any
	if decision == "UNDER_REVIEW" {
		resolution = nil
	} else {
		resolution = evidenceRef
	}
	_, err = tx.ExecContext(ctx, `UPDATE academy_assessment_appeals
 SET state=$2,independent_reviewer_id=$3::uuid,review_evidence_ref=$4,
 resolved_at=CASE WHEN $2 IN ('UPHELD','REJECTED') THEN now() ELSE NULL END
 WHERE id=$1::uuid`, appealID, decision, actor, resolution)
	if err != nil {
		return err
	}
	if err = academyEvent(ctx, tx, actor, "academy/appeal/"+appealID,
		"academy.assessment.appeal.reviewed", map[string]any{"decision": decision, "evidence_ref": evidenceRef}); err != nil {
		return err
	}
	return tx.Commit()
}
